package util

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/resty.v1"

	"github.com/adevinta/vulcan-checks-bsys/config"
	"github.com/adevinta/vulcan-checks-bsys/manifest"
)

// RegistryConfig stores the name a credentials for a registry.
type RegistryConfig struct {
	RegistryServer string
	RegistryUser   string
	RegistryPass   string
}

// BuildImage builds and image given a tar, a list of tags and labels.
func BuildImage(tarFile io.Reader, tags []string, labels map[string]string) (response string, err error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	buildOptions := types.ImageBuildOptions{
		Tags:   tags,
		Labels: labels,
	}

	re, err := cli.ImageBuild(ctx, tarFile, buildOptions)
	if err != nil {
		return "", err
	}

	// NOTE: shouldn't we be passing a log.Logger like in PushImage func?
	lines, err := readDockerOutput(re.Body, nil)
	return strings.Join(lines, "\n"), err
}

// RunCheckImage creates an runs a check in a container.
func RunCheckImage(imgName string, env []string) error {
	envCli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	cli := envCli
	ctx := context.Background()

	info, _, err := envCli.ImageInspectWithRaw(ctx, imgName)
	if err != nil {
		return err
	}

	cfg := &container.Config{
		Image: imgName,
		Cmd: []string{
			strings.Join([]string(info.Config.Cmd), " "),
			"-t",
		},
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		Env:          env,
	}

	r, err := cli.ContainerCreate(ctx, cfg, nil, nil, "")
	if err != nil {
		return err
	}

	attResp, err := envCli.ContainerAttach(ctx, r.ID, types.ContainerAttachOptions{Stdout: true,
		Stderr: true,
		Stdin:  true,
		Stream: true,
		Logs:   true,
	})
	if err != nil {
		return err
	}
	defer attResp.Close()

	if err = cli.ContainerStart(ctx, r.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	_, err = io.Copy(os.Stdout, attResp.Reader)
	if err != nil {
		return err
	}

	_, err = cli.ContainerWait(ctx, r.ID)

	return err
}

// PushImage pushes a image to a given repository using provided credentials.
func PushImage(imageName string, logger *log.Logger) (response string, err error) {
	envCli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}

	cli := envCli
	ctx := context.Background()

	username, password := getDockerCredentials()
	cfg := types.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: config.Cfg.DockerRegistry,
	}

	buf, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	encodedAuth := base64.URLEncoding.EncodeToString(buf)
	pushOpts := types.ImagePushOptions{
		RegistryAuth: encodedAuth,
	}

	r, err := cli.ImagePush(ctx, imageName, pushOpts)
	if err != nil {
		return "", err
	}

	lines, err := readDockerOutput(r, logger)
	return strings.Join(lines, "\n"), err
}

func readDockerOutput(r io.Reader, logger *log.Logger) (lines []string, err error) {
	reader := bufio.NewReader(r)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Function will return error only if it's not a EOF.
				err = nil
			}
			return lines, err
		}

		lines = append(lines, line)

		msg, err := parsePushImageResultLine(line)
		if err != nil {
			return nil, err
		}

		if msg.ErrorDetail != nil {
			err = errors.New(msg.ErrorDetail.Message)
			if logger != nil {
				logger.Printf(line)
			}
			return nil, err
		}

		if logger != nil {
			logger.Printf(line)
		}
	}
}

type pushImgRespResp struct {
	Status      string               `json:"status,omitempty"`
	ErrorDetail *types.ErrorResponse `json:"errorDetail,omitempty"`
}

func parsePushImageResultLine(line string) (imgResp *pushImgRespResp, err error) {
	imgResp = &pushImgRespResp{}
	err = json.Unmarshal([]byte(line), imgResp)
	return
}

// BuildTarFromDir builds  a tar file in memory given a sourcedir.
func BuildTarFromDir(sourcedir string) (tarContents *bytes.Buffer, err error) {
	dir, err := os.Open(path.Clean(sourcedir))
	if err != nil {
		return nil, err
	}
	defer dir.Close() // nolint: errcheck

	files, err := dir.Readdir(0)
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	tarfileWriter := tar.NewWriter(&output)
	defer tarfileWriter.Close() // nolint: errcheck

	err = addDir(sourcedir, "", tarfileWriter, files)
	return &output, err
}

// NOTE: consider split into functions.
func addDir(sourceDir string, currentPath string, writer *tar.Writer, finfo []os.FileInfo) error {
	for _, file := range finfo {
		tarPath := path.Join(currentPath, file.Name())

		// If file is a dir recursion.
		if file.IsDir() {
			absPath := path.Join(sourceDir, tarPath)
			dir, err := os.Open(absPath)
			if err != nil {
				return err
			}

			files, err := dir.Readdir(0)
			if err != nil {
				return err
			}

			err = addDir(sourceDir, tarPath, writer, files)
			if err != nil {
				return err
			}
		} else {
			// File is not a dir, add to tar.
			h, err := tar.FileInfoHeader(file, tarPath)
			if err != nil {
				return err
			}

			h.Name = tarPath
			if err = writer.WriteHeader(h); err != nil {
				return err
			}

			absFilePath := path.Join(sourceDir, tarPath)

			var content []byte
			content, err = ioutil.ReadFile(absFilePath)
			if err != nil {
				return err
			}

			if _, err = writer.Write(content); err != nil {
				return err
			}
		}
	}

	return nil
}

// ImageTagsInfo represents the info returned by
// "/[reponame]/[imagename]/tags/list" rest query.
type ImageTagsInfo struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// ImageVersionInfo stores the info about version of a check, that is the last
// commit affecting the check, and the sdk version, witch in turn, the last
// commit in the master branch of the sdk.
type ImageVersionInfo struct {
	LastModified time.Time
	Commit       string
	SDKVersion   string
	Manifest     manifest.Data
}

func setupAPICred(client *resty.Client) {
	username, password := getDockerCredentials()
	client.SetBasicAuth(username, password)
}

func getDockerCredentials() (string, string) {
	user := config.Cfg.DockerRegistryUser
	pwd := config.Cfg.DockerRegistryPwd
	if user == "" || pwd == "" {
		user, pwd = askForCredentials()
	}
	return user, pwd
}

func askForCredentials() (username, password string) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		// NOTE: consider using a logger.
		fmt.Printf("Can not get artifactory credentials")
		// NOTE: consider raising an error instead of panic.
		panic(err)
	}

	reader := bufio.NewReader(tty)

	fmt.Print("Enter Username for artifactory: ")
	username, err = reader.ReadString('\n')
	if err != nil {
		// NOTE: consider using a logger.
		fmt.Printf("Can not get artifactory credentials")
		// NOTE: consider raising an error instead of panic.
		panic(err)
	}

	fmt.Print("Enter Password for artifactory: ")
	bytePassword, err := terminal.ReadPassword(int(tty.Fd()))
	if err != nil {
		// NOTE: consider using a logger.
		fmt.Printf("Can not get artifactory credentials")
		// NOTE: consider raising an error instead of panic.
		os.Exit(1)
	}

	password = string(bytePassword)
	// NOTE: is it necessary to trim spaces, considering that a password may contain spaces?
	return strings.TrimSpace(username), strings.TrimSpace(password)
}

// FetchImagesInfo get information about images deployed in artifactory.
func FetchImagesInfo(image string) (result ImageTagsInfo, err error) {
	restyClient := resty.New()
	client := restyClient.SetHostURL(config.Cfg.DockerAPIBaseURL)
	setupAPICred(client)

	tagsPath := fmt.Sprintf("/%v/%v/tags/list", config.Cfg.VulcanChecksRepo, image)

	r := client.R()
	response, err := r.Get(tagsPath)

	if err != nil {
		return
	}

	if response.RawResponse.StatusCode == http.StatusOK {
		err = json.Unmarshal(response.Body(), &result)
		return
	}

	if response.RawResponse.StatusCode == http.StatusNotFound {
		result.Name = config.Cfg.VulcanChecksRepo + "/" + image
		return
	}

	err = fmt.Errorf("error returned by query %s, status: %s", response.Request.URL, response.RawResponse.Status)
	return
}

// FetchRepositories gets all docker repositories in artifactory. this can
// potentially return a lot of values but, unfortunately by now, we didn't found
// any way for querying artifactory only for the vulcan-checks folder.
func FetchRepositories() ([]string, error) {
	reps := struct {
		Repositories []string `json:"repositories"`
	}{}
	restyClient := resty.New()
	client := restyClient.SetHostURL(config.Cfg.DockerAPIBaseURL)
	setupAPICred(client)
	r := client.R()
	response, err := r.Get("/_catalog")
	// NOTE: consider using Logger.
	fmt.Printf("\nrequest path: %s\n", response.Request.URL)
	if err != nil {
		return nil, err
	}

	if response.RawResponse.StatusCode == http.StatusOK {
		err = json.Unmarshal(response.Body(), &reps)
		return reps.Repositories, err
	}

	if response.RawResponse.StatusCode == http.StatusNotFound {
		return nil, errors.New("no docker repositories found")
	}

	err = fmt.Errorf("error returned by query %s, status: %s", response.Request.URL, response.RawResponse.Status)
	return nil, err
}

type imageTagPayload struct {
	Properties map[string][]string
}

// FetchImageTagInfo get information about a concrete image version deployed in
// artifactory.
func FetchImageTagInfo(image string, tag string) (ImageVersionInfo, error) {
	result := ImageVersionInfo{}
	restyClient := resty.New()
	client := restyClient.SetHostURL(config.Cfg.DockerAPIBaseExtendedURL)
	setupAPICred(client)
	tagsPath := fmt.Sprintf("%s/%s/%s/manifest.json?properties", config.Cfg.VulcanChecksRepo, image, tag)
	r := client.R()

	response, err := r.Get(tagsPath)
	if err != nil {
		return result, err
	}
	if response.RawResponse.StatusCode == http.StatusNotFound {
		result.Commit = ""
		return result, err
	}
	if response.RawResponse.StatusCode != http.StatusOK {
		err = fmt.Errorf("%s", response.RawResponse.Status)
		return result, err
	}

	lastModified, err := http.ParseTime(response.Header().Get("Last-Modified"))
	if err != nil {
		return result, err
	}

	result.LastModified = lastModified

	payload := &imageTagPayload{}
	err = json.Unmarshal(response.Body(), payload)
	if err != nil {
		return result, err
	}

	commits, exists := payload.Properties["docker.label.commit"]
	if !exists {
		// NOTE: consider using Logger.
		fmt.Printf("Label docker.label.commit doesn't exist in image %s:%s", image, tag)
		result.Commit = ""
	} else {
		// Only the first value should be meaningful.
		result.Commit = commits[0]
	}

	sdkVersions, exists := payload.Properties["docker.label.sdk-version"]
	if !exists {
		// NOTE: consider using Logger.
		fmt.Printf("Label docker.label.sdk-version doesn't exist in image %s:%s", image, tag)
		result.SDKVersion = ""
	} else {
		// Only the first value should be meaningful.
		result.SDKVersion = sdkVersions[0]
	}

	rawManifest, exists := payload.Properties["docker.label.manifest"]
	if !exists {
		fmt.Printf("Label docker.label.manifest doesn't exist in image %s:%s", image, tag)
	} else {
		// Only the first value should be meaningful.
		err = json.Unmarshal([]byte(rawManifest[0]), &result.Manifest)
	}

	return result, err
}

// GetCurrentSDKVersion get the current sdk version. The function supposes the
// git repo of the sdk is already cloned locally.
func GetCurrentSDKVersion() (string, error) {
	cmd := fmt.Sprintf("go list -m %s | sed 's=-= =g' | awk '{print $NF}'", config.Cfg.SDKPath)
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DirLastCommmit stores information about last commit as returned by git log.
type DirLastCommmit struct {
	Path   string
	Commit string
}

// GetLastCommitForDir returns information about last commit in dir.
func GetLastCommitForDir(dir string) (DirLastCommmit, error) {
	return GetLastCommitForDirInRepo(dir, "")
}

// GetLastCommitForDirInRepo returns information about last commit in dir.
func GetLastCommitForDirInRepo(dir, repoPath string) (DirLastCommmit, error) {
	cmdName := "git"
	cmdArgs := []string{"log", "--oneline", dir}
	cmd := exec.Command(cmdName, cmdArgs...)
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	cmd.Env = os.Environ()
	cmdOut, err := cmd.CombinedOutput()
	if err != nil {
		return DirLastCommmit{}, err
	}

	// Get results.
	commit, err := parseGitLogLine(string(cmdOut))
	if err != nil {
		return DirLastCommmit{}, err
	}

	return DirLastCommmit{Commit: commit, Path: dir}, nil
}

func parseGitLogLine(gitLogOutput string) (commit string, err error) {
	// Example:  "137559c Fix error in  vulcan-is-exposed. (#5)"
	gitLines := strings.Split(gitLogOutput, "\n")
	if len(gitLines) < 1 {
		err = fmt.Errorf("Format error in log result: %s", gitLogOutput)
		return
	}

	gitLine := gitLines[0]
	parts := strings.Fields(gitLine)
	if len(parts) < 1 {
		err = fmt.Errorf("Format error in log result: %s", gitLogOutput)
		return
	}

	commit = parts[0]
	return
}

// GoBuildDir execute `go build .` in a process setting the Dir of the process to checkDir param.
// Also sets the GOOS var to linux.
func GoBuildDir(checkDir string) error {
	args := []string{"build", "-a", "-ldflags", "-extldflags -static", "."}
	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOOS=linux", "CGO_ENABLED=0")
	cmd.Dir = checkDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GoTestDir execute `go test .` in a process setting the Dir of the process to checkDir param.
func GoTestDir(checkDir string) error {
	args := []string{"test"}
	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	cmd.Dir = checkDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetLatestTag given an array of tags returns
// the last tag without considering alfanumerics values, for instance
// in for this input ["10","7","something"] will return 10.
// will return false if len of input is zero or no numeric values are present in the array.
func GetLatestTag(tags []string) (latestTag string, found bool) {
	var last int64
	for _, tag := range tags {
		// Take into account only tags that are valid integers.
		ver, err := strconv.ParseInt(tag, 0, 64)
		if err != nil {
			continue
		}

		// If we are here at least exists one tag that is a valid integer.
		found = true

		if ver > last {
			last = ver
		}
	}

	latestTag = strconv.FormatInt(last, 10)
	return
}
