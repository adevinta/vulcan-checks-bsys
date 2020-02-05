package config

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"fmt"
	"github.com/BurntSushi/toml"
	"os/user"
)

// Cfg contains the loaded confing.
var Cfg Config

// Config stores the configuration needed by the build system.
type Config struct {
	DockerAPIBaseURL         string `toml:"docker_api_base_url"`
	DockerAPIBaseExtendedURL string `toml:"docker_api_base_extended_url"`
	DockerRegistryUser       string `toml:"docker_registry_user"`
	DockerRegistryPwd        string `toml:"docker_registry_pwd"`
	SDKPath                  string `toml:"docker_sdk_path"`
	DockerRegistry           string `toml:"docker_registry_pwd"`
	VulcanChecksRepo         string `toml:"vulcan_checks_repo"`
	PersistencePro           string `toml:"persistence_pro"`
	PersistencePre           string `toml:"persistence_pre"`
	PersistenceDev           string `toml:"persistence_dev"`
}

// LoadFrom loads the config from the specified file path.
func LoadFrom(path string) error {
	if path == "" {
		usr, err := user.Current()
		if err != nil {
			return fmt.Errorf("Can't get current user:%+v", err)
		}
		path = filepath.Join(usr.HomeDir, ".vulcan-checks-bsys.toml")

	}
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = toml.Unmarshal(contents, &Cfg)
	if err != nil {
		return err
	}
	// Override with the parameters that can be specified by env vars if needed.
	if os.Getenv("DOCKER_REGISTRY_USER") != "" {
		Cfg.DockerRegistryUser = os.Getenv("DOCKER_REGISTRY_USER")
	}

	if os.Getenv("DOCKER_REGISTRY_PWD") != "" {
		Cfg.DockerRegistryPwd = os.Getenv("DOCKER_REGISTRY_PWD")
	}
	if os.Getenv("DOCKER_REGISTRY") != "" {
		Cfg.DockerRegistry = os.Getenv("DOCKER_REGISTRY")
	}
	return nil
}
