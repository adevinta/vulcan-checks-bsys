/*
Copyright 2021 Adevinta
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adevinta/vulcan-checks-bsys/config"
	"github.com/adevinta/vulcan-checks-bsys/util"
)

const (
	usage = `usage: detect-images -c config_file_path baseDirPath ResultFilePath. 
If the config file path is not specified it defaults to ~/.vulcan-checks-bsys.toml
baseDirPath must be relative to the git repo.`

	buildBranchEnvVar  = "TRAVIS_BRANCH"
	forceBuildEnvVar   = "FORCE_BUILD"
	forceBuildAllToken = "ALL"
	prodBranchName     = "master"
	imgNameDevSuffix   = "experimental"
	configFlagUsage    = "Path to the configuration file"
)

var (
	logger          *log.Logger
	logWriter       = os.Stdout
	cfg             string
	forceBuildImage = ""
)

func init() {
	forceBuildImage = os.Getenv(forceBuildEnvVar)
	logger = log.New(
		logWriter,
		"detect-images",
		log.Lshortfile,
	)
}

func main() {
	flag.StringVar(&cfg, "c", "", configFlagUsage)
	flag.Parse()
	err := config.LoadFrom(cfg)
	if err != nil {
		logger.Fatal(err)
	}
	if len(flag.Args()) < 2 {
		fmt.Println(usage)
		return
	}
	args := flag.Args()
	baseDir := args[0]
	resultFilePath := args[1]
	if forceBuildImage == "" {
		err = detectImages(baseDir, resultFilePath, false)
	} else if forceBuildImage == forceBuildAllToken {
		logger.Print("Rebuilding all images")
		err = detectImages(baseDir, resultFilePath, true)
	} else {
		err = forceDetectOneImage(baseDir, resultFilePath, forceBuildImage)
	}

	if err != nil {
		logger.Fatal(err)
	}
}

func forceDetectOneImage(baseDir, resultFilePath, imageName string) error {
	branchName := os.Getenv(buildBranchEnvVar)
	logger.Printf("Build branch name: %s", branchName)
	env := ""
	if branchName != prodBranchName {
		env = imgNameDevSuffix
	}
	f, err := os.Open(baseDir)
	if err != nil {
		return err
	}
	entries, err := f.Readdir(0)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() && imageName == entry.Name() {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("error, check: %s, not found", imageName)
	}
	dir := filepath.Join(baseDir, imageName)
	commitInfos, err := getLastCommitForDirs([]string{dir})
	if err != nil {
		return err
	}
	dirs, err := getImagesToBuild(commitInfos, env, true)
	if err != nil {
		return err
	}
	result := strings.Join(dirs, "\n")
	logger.Printf("Image to build:\n%v", result)
	return ioutil.WriteFile(resultFilePath, []byte(result), 0644)

}

func detectImages(baseDir, resultFilePath string, force bool) error {
	branchName := os.Getenv(buildBranchEnvVar)
	logger.Printf("Build branch name: %s", branchName)
	env := ""
	if branchName != prodBranchName {
		env = imgNameDevSuffix
	}
	dirs, err := getDirsUnder(baseDir)
	if err != nil {
		return err
	}

	commitInfos, err := getLastCommitForDirs(dirs)
	if err != nil {
		return err
	}
	logger.Printf("commitInfos:\n%+v", commitInfos)

	dirs, err = getImagesToBuild(commitInfos, env, force)
	if err != nil {
		return err
	}

	result := strings.Join(dirs, "\n")
	logger.Printf("Images to build:\n%v", result)
	return ioutil.WriteFile(resultFilePath, []byte(result), 0644)
}
func getDirsUnder(dir string) (dirs []string, err error) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}

	entries, err := f.Readdir(0)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, path.Join(dir, entry.Name()))
		}
	}

	return
}

func getImagesToBuild(dirsInfo []util.DirLastCommmit, env string, force bool) (dirs []string, err error) {
	sdkVer, err := util.GetCurrentSDKVersion()
	if err != nil {
		return nil, fmt.Errorf("Error getting current sdk version.Details: %v", err)
	}
	logger.Printf("SDK version: %s", sdkVer)

	for _, dirInfo := range dirsInfo {
		// The imgName start value is the name of the directory of the last commit.
		imgName := path.Base(dirInfo.Path)
		if env != "" {
			imgName = fmt.Sprintf("%s-%s", imgName, env)
		}

		imgInfo, err := util.FetchImagesInfo(imgName)
		if err != nil {
			return nil, err
		}

		tag, found := util.GetLatestTag(imgInfo.Tags)
		if found {
			// NOTE: This can be improved!!. We don't need to fetch image info when force is true.
			imageInfo, err := util.FetchImageTagInfo(imgInfo.Name, tag)
			if err != nil {
				return nil, err
			}
			if imageInfo.Commit != dirInfo.Commit || imageInfo.SDKVersion != sdkVer || force {
				dirs = append(dirs, dirInfo.Path+":"+incrementTagVersion(tag)+":"+dirInfo.Commit)
			}
		} else {
			dirs = append(dirs, dirInfo.Path+":"+incrementTagVersion(tag)+":"+dirInfo.Commit)
		}
	}

	return dirs, nil
}

func incrementTagVersion(tag string) string {
	v, err := strconv.ParseInt(tag, 0, 64)
	if err != nil {
		return "0"
	}
	v++
	return strconv.FormatInt(v, 10)
}

func getLastCommitForDirs(dirs []string) (commitInfos []util.DirLastCommmit, err error) {
	for _, dir := range dirs {
		commitInfo, err := util.GetLastCommitForDir(dir)
		if err != nil {
			return nil, err
		}

		commitInfos = append(commitInfos, commitInfo)
	}

	return commitInfos, nil
}
