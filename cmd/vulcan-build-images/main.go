package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	sdkconfig "github.com/adevinta/vulcan-check-sdk/config"
	"github.com/adevinta/vulcan-checks-bsys/config"
	"github.com/adevinta/vulcan-checks-bsys/manifest"
	"github.com/adevinta/vulcan-checks-bsys/persistence"
	"github.com/adevinta/vulcan-checks-bsys/util"
)

const (
	usage             string = "usage: \n"
	buildBranchEnvVar string = "TRAVIS_BRANCH"
	prodBranchName    string = "master"
	imgNameDevSuffix  string = "-experimental"
	manifestFileName  string = "manifest.toml"

	forceFlagUsage string = `Path to a directory of the repo that contains a check. 
Builds check docker image locally, without publishing it to the docker repository.
This flag can not be used in conjunction with -i flag`

	imageFlagUsage = `Path to a file containing one text line for each directory with a dockerfile to build.
This flag can not be used in conjunction with -f flag`

	publishFlagUsage = `Queries the docker repository for the last version of all checks
and publishes them to the provided persistence service endpoint. All checks, both experimental, 
and not experimental are published. 
When the flag is set, the env var DOCKER_REGISTRY_ENV must be specified. 
Example:
export DOCKER_REGISTRY_ENV=DOCKER_REGISTRY 
DOCKER_REGISTRY="docker-registry.example.com" vulcan-build-images -p https://vulcan-persistence.example.com/ `

	runFlagUsage = `Same as force flag but also runs resulting docker image
with -t flag and sets env vars with values defined in the corresponding local.toml.`
	configFlagUsage = `Path to the configuration file, if it's not provided it defaults to ~/.vulcan-checks-bsys.toml`
)

var (
	logger      *log.Logger
	logWriter   = os.Stdout
	buildBranch = os.Getenv(buildBranchEnvVar)
	imagesFile  string
	force       string
	publish     string
	run         string
	cfg         string
)

func init() {
	name := os.Args[0]
	logger = log.New(
		logWriter,
		fmt.Sprintf("%s: ", name),
		log.Lshortfile,
	)
}
func main() {
	mustParseFlags()
	var err error
	if force != "" {
		_, err = forceBuild(force)
	} else if run != "" {
		err = forceRun(run)
	} else if publish != "" {
		err = publishChecks(publish)
	} else if imagesFile != "" {
		err = buildImages(imagesFile)
	} else {
		err = errors.New("You must specify at least one flag")
	}
	if err != nil {
		log.Fatal(err)
	}
	err = config.LoadFrom(cfg)
	if err != nil {
		log.Fatal(err)
	}
}

func mustParseFlags() {
	// Allow setting the flag params in tests
	if !flag.Parsed() {
		flag.StringVar(&imagesFile, "i", "", imageFlagUsage)
		flag.StringVar(&force, "f", "", forceFlagUsage)
		flag.StringVar(&publish, "p", "", publishFlagUsage)
		flag.StringVar(&run, "r", "", runFlagUsage)
		flag.StringVar(&cfg, "c", "", configFlagUsage)
		flag.Parse()
	}

	if imagesFile == "" && force == "" && publish == "" && run == "" {
		printHelp()
		os.Exit(1)
	}

	if imagesFile != "" && force != "" {
		printHelp()
		os.Exit(1)
	}

	err := config.LoadFrom(cfg)
	if err != nil {
		fmt.Printf("%+v", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(usage)
	flag.PrintDefaults()
}

type checkImageInfo struct {
	checktypeName string // e.g.: vulcan-wpscan-experimental
	imagePath     string // e.g.: cmd/vulcan-wpscan
	imageName     string // e.g.: container.example.com/vulcan-checks/vulcan-wpscan-experimental
	manifest      manifest.Data
}

func buildImages(imagesFilePath string) error {
	images, err := readImageDirs(imagesFilePath)
	if err != nil {
		return err
	}

	if len(images) < 1 {
		logger.Printf("No images to build")
		return nil
	}

	logger.Printf("Number of images to build: %v", len(images))

	imagesToPush, err := processImages(images)
	if err != nil {
		return err
	}
	return pushImagesAndChecktypes(imagesToPush)
}

// NOTE: in some other areas we are using something like this to read lines.
// Haven't done research about wheter it's better or not.
/*
// ReadLines reads a whole file into memory
// and returns a slice of its lines.
func ReadLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
*/
func publishChecks(endpoint string) error {
	repos, err := util.FetchRepositories()
	if err != nil {
		return err
	}
	checks := []string{}
	// Get vulcan checks from all the docker images in artifactory.
	for _, val := range repos {
		if strings.HasPrefix(val, "vulcan-checks/") {
			checkNameParts := strings.Split(val, "/")
			if len(checkNameParts) > 0 {
				checks = append(checks, checkNameParts[1])
			}
		}
	}

	var imagesToPub []checkImageInfo
	for _, name := range checks {
		imgInfo, err := util.FetchImagesInfo(name)
		if err != nil {
			return err
		}
		tag, found := util.GetLatestTag(imgInfo.Tags)
		if !found {
			// If the docker image in artifactory for the checks doesn't have
			// a valid tag it shouldn't be published.
			logger.Printf("Skiping image because not valid tag present. Image info:%v", imgInfo)
			continue
		}

		imageName := buildImageName(name, tag)
		repoInfo, err := util.FetchImageTagInfo(name, tag)
		if err != nil {
			return err
		}
		// Description is a mandatory field, if empty,
		// means the image doesn't have yet the manifest info stored in artifactory.
		if repoInfo.Manifest.Description == "" {
			logger.Printf("There is no manifest info in artifactory for image:%s\n", name)
		}
		info := checkImageInfo{
			checktypeName: name,
			imagePath:     imageName,
			manifest:      repoInfo.Manifest,
		}
		imagesToPub = append(imagesToPub, info)

	}
	pClient := persistence.NewClient(endpoint)
	for _, img := range imagesToPub {
		assetsTypes, err := img.manifest.AssetTypes.Strings()
		if err != nil {
			return err
		}
		resp, err := pClient.PublishChecktype(persistence.Checktype{
			Name:         img.checktypeName,
			Description:  img.manifest.Description,
			Image:        img.imagePath,
			Options:      img.manifest.Options,
			RequiredVars: img.manifest.RequiredVars,
			QueueName:    img.manifest.QueueName,
			Timeout:      img.manifest.Timeout,
			Assets:       assetsTypes,
		})
		if err != nil {
			return err
		}
		logger.Printf("Checktype %v published to the persistence service. Checktype data returned:\n %+v", img.imageName, resp)
	}

	return nil
}

func readImageDirs(path string) (paths []string, err error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	nonFiltered := strings.Split(string(content), "\n")
	// Filter empty lines.
	for _, line := range nonFiltered {
		if strings.Trim(line, " ") != "" {
			paths = append(paths, line)
		}

	}
	return
}

func processImages(images []string) (imagesToPush []checkImageInfo, err error) {
	sdkVer, err := util.GetCurrentSDKVersion()
	if err != nil {
		return nil, fmt.Errorf("Error getting current sdk version.Details: %v", err)
	}
	env := ""
	if buildBranch != prodBranchName {
		env = imgNameDevSuffix
	}
	for _, image := range images {
		imagePath, tag, commit := parseImgInfo(image)
		imageName := buildImageNameWithEnvSuffix(path.Base(imagePath), tag)
		m, err := manifest.Read(path.Join(imagePath, manifestFileName))
		if err != nil {
			return nil, err
		}
		i := checkImageInfo{
			imageName:     imageName,
			imagePath:     imagePath,
			checktypeName: path.Base(imagePath) + env,
			manifest:      m,
		}
		logger.Printf("Running go build for dir %s", imagePath)
		if err = util.GoBuildDir(i.imagePath); err != nil {
			return nil, err
		}
		logger.Printf("Building image for dir %s", i.imagePath)
		contents, err := util.BuildTarFromDir(i.imagePath)
		if err != nil {
			return nil, err
		}
		// Marshall manifest.
		man, err := json.Marshal(i.manifest)
		if err != nil {
			return nil, err
		}
		logOutput, err := util.BuildImage(contents, []string{i.imageName}, map[string]string{
			"commit":      commit,
			"sdk-version": sdkVer,
			"manifest":    string(man),
		})
		if err != nil {
			logger.Printf("Output of the failed docker build: %s", logOutput)
			return nil, err
		}

		logger.Printf("Docker image built")
		imagesToPush = append(imagesToPush, i)
	}

	return imagesToPush, nil
}

func parseImgInfo(imgInfo string) (path, tag, commit string) {
	parts := strings.Split(imgInfo, ":")
	path = parts[0]
	tag = parts[1]
	commit = parts[2]
	return
}

func buildImageNameWithEnvSuffix(imgName, tag string) string {
	if buildBranch != prodBranchName {
		imgName = imgName + imgNameDevSuffix
	}
	return buildImageName(imgName, tag)
}
func buildImageName(imgName, tag string) string {
	return fmt.Sprintf("%s/%s/%s:%s", config.Cfg.DockerRegistry, config.Cfg.VulcanChecksRepo, imgName, tag)
}
func pubChecktypeToPersistence(checkName string, metadata manifest.Data, imagePath string, fail bool, envs ...string) error {
	for _, persistenceEndPoint := range envs {
		// Only publish checktypes to valid endpoints
		if persistenceEndPoint == "" {
			continue
		}
		logger.Printf("Publishing image to a new checktype in: %v", persistenceEndPoint)
		pClient := persistence.NewClient(persistenceEndPoint)
		assetTypes, err := metadata.AssetTypes.Strings()
		if err != nil {
			return err
		}
		resp, err := pClient.PublishChecktype(persistence.Checktype{
			Name:         checkName,
			Description:  metadata.Description,
			Image:        imagePath,
			Options:      metadata.Options,
			RequiredVars: metadata.RequiredVars,
			QueueName:    metadata.QueueName,
			Timeout:      metadata.Timeout,
			Assets:       assetTypes,
		})
		if err != nil && fail {
			return err
		}
		if err != nil && !fail {
			fmt.Printf("error pushing to secondary persistence:%+s, the process will continue", persistenceEndPoint)
			continue
		}

		logger.Printf("Checktype %v published to the persistence service. Checktype data returned:\n %+v", imagePath, resp)
	}
	return nil
}

func pushImagesAndChecktypes(imagesToPush []checkImageInfo) error {
	for _, i := range imagesToPush {
		logger.Printf("Pushing image %s", i.imageName)
		_, err := util.PushImage(i.imageName, nil)
		if err != nil {
			return err
		}
		logger.Printf("Docker image %s pushed", i.imageName)
		if buildBranch != prodBranchName {
			// In feature branches only publish checktypes to dev envs.
			// For the primary envs we fail if there is an error publising the check to any of them.
			err = pubChecktypeToPersistence(i.checktypeName, i.manifest, i.imageName, true, config.Cfg.PrimaryDevBranchEnvs...)
			if err == nil {
				// For the primary envs we fail if there is an error publising the check to any of them.
				err = pubChecktypeToPersistence(i.checktypeName, i.manifest, i.imageName, false, config.Cfg.PrimaryDevBranchEnvs...)
			}
		} else {
			// In master branch publish checktypes to all the environments.
			primaryEnvs := append(config.Cfg.PrimaryMasterBranchEnvs, config.Cfg.PrimaryDevBranchEnvs...)
			err = pubChecktypeToPersistence(i.checktypeName, i.manifest, i.imageName, true, primaryEnvs...)
			if err == nil {
				secondaryEnvs := append(config.Cfg.SecondaryMasterBranchEnvs, config.Cfg.SecondaryDevBranchEnvs...)
				err = pubChecktypeToPersistence(i.checktypeName, i.manifest, i.imageName, false, secondaryEnvs...)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func forceRun(imagePath string) error {
	var (
		err       error
		imageName string
	)

	if imageName, err = forceBuild(imagePath); err != nil {
		return err
	}
	var env []string
	// Setup container env by reading, if present, local.toml file of the check dir.
	cpath := path.Join(imagePath, "local.toml")
	if _, err := os.Stat(cpath); !os.IsNotExist(err) {
		c, err := sdkconfig.LoadConfigFromFile(cpath)
		if err != nil {
			return err
		}

		allowPrivateIPs := true
		if c.AllowPrivateIPs != nil {
			allowPrivateIPs = *c.AllowPrivateIPs
		}
		if c.Log.LogLevel != "" {
			env = append(env, "VULCAN_CHECK_LOG_LVL="+c.Log.LogLevel)
		}
		// NOTE: the name of the env vars should be read from public constants of the sdk.
		env = append(env, "VULCAN_CHECK_TARGET="+c.Check.Target)
		env = append(env, "VULCAN_CHECK_OPTIONS="+c.Check.Opts)
		env = append(env, "VULCAN_ALLOW_PRIVATE_IPS="+strconv.FormatBool(allowPrivateIPs))

		for k, v := range c.RequiredVars {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return util.RunCheckImage(imageName, env)
}

func forceBuild(imagePath string) (string, error) {
	env := ""
	if buildBranch != prodBranchName {
		env = imgNameDevSuffix
	}
	logger.Printf("Reading manifest for dir %s", imagePath)
	// Run go build in the check dir.
	if err := goBuild(imagePath); err != nil {
		return "", err
	}
	// Build tar file with docker image contents.
	logger.Printf("Building image for dir %s", imagePath)
	contents, err := util.BuildTarFromDir(imagePath)
	if err != nil {
		return "", err
	}

	imageName := path.Base(imagePath)
	imageName = fmt.Sprintf("%s%s", imageName, env)
	r, err := util.BuildImage(contents, []string{imageName}, map[string]string{})
	if err != nil {
		return "", err
	}
	logger.Printf("Docker image built, build log:\n%s\nImage name %s:", r, imageName)
	return imageName, nil
}

func goBuild(imagePath string) error {
	logger.Printf("Running go build for dir %s", imagePath)
	return util.GoBuildDir(imagePath)
}
