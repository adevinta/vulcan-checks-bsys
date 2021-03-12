/*
Copyright 2021 Adevinta
*/

package util

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/adevinta/vulcan-checks-bsys/config"
)

var (
	update = flag.Bool("update", false, "update golden files")
)

func writeJSONResponse(w http.ResponseWriter, code int, r string, headers map[string]string) {
	for v, k := range headers {
		w.Header().Set(v, k)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, "%s", r)
}

func buildFakeDockerRegistry(result string) *httptest.Server {
	return buildFakeDockerRegistryWithHeaders(result, nil)
}

func buildFakeDockerRegistryWithHeaders(result string, headers map[string]string) *httptest.Server {
	handle := func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusOK, result, headers)
	}
	return httptest.NewServer(http.HandlerFunc(handle))
}
func TestFetchImagesInfo(t *testing.T) {
	type args struct {
		image string
	}
	tests := []struct {
		name        string
		args        args
		apiResponse string
		wantResult  ImageTagsInfo
		wantErr     bool
	}{
		{
			name:        "HappyPath",
			args:        args{image: "vulcan-exposed-db"},
			apiResponse: "{\"name\":\"vulcan-checks/testcheck-experimental\",\"tags\":[ \"0\", \"0.0.0\"]}",
			wantResult: ImageTagsInfo{
				Name: "vulcan-checks/testcheck-experimental",
				Tags: []string{"0", "0.0.0"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		// Set user and pwd vars to avoid asking for credentials.
		config.Cfg.DockerRegistryUser = "oneuser"
		config.Cfg.DockerRegistryPwd = "secret"
		s := buildFakeDockerRegistry(tt.apiResponse)
		defer s.Close()
		config.Cfg.DockerAPIBaseURL = s.URL
		t.Run(tt.name, func(t *testing.T) {
			gotResult, err := FetchImagesInfo(tt.args.image)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchImagesInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResult, tt.wantResult) {
				t.Errorf("FetchImagesInfo() = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestFetchImageTagInfo(t *testing.T) {
	type args struct {
		image string
		tag   string
	}
	tests := []struct {
		name               string
		args               args
		apiResponse        string
		apiResponseHeaders map[string]string
		wantResult         ImageVersionInfo
		wantErr            bool
	}{
		{
			name:               "HappyPath",
			args:               args{image: "vulcan-checks/vulcan-exposed-db", tag: "0.0.1"},
			apiResponse:        "{\"properties\":{\"docker.label.commit\": [\"01234a\"],\"docker.label.sdk-version\": [\"8e938a5\"]}}",
			apiResponseHeaders: map[string]string{"Last-Modified": "Wed, 25 May 2017 14:25:03 GMT"},
			wantResult: ImageVersionInfo{
				Commit:       "01234a",
				LastModified: time.Date(2017, time.May, 25, 14, 25, 3, 0, time.UTC),
				SDKVersion:   "8e938a5",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		s := buildFakeDockerRegistryWithHeaders(tt.apiResponse, tt.apiResponseHeaders)
		defer s.Close()
		config.Cfg.DockerRegistryPwd = "pwd"
		config.Cfg.DockerRegistryUser = "user"
		config.Cfg.DockerAPIBaseExtendedURL = s.URL
		t.Run(tt.name, func(t *testing.T) {
			gotResult, err := FetchImageTagInfo(tt.args.image, tt.args.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchImageTagInfo() error = %+v, wantErr %+v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResult, tt.wantResult) {
				t.Errorf("FetchImageTagInfo() = %+v, want %+v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestBuildTarFromDir(t *testing.T) {
	type args struct {
		sourcedir string
	}
	tests := []struct {
		goldenPath      string
		name            string
		args            args
		wantTarContents string
		wantErr         bool
	}{
		{
			name:       "HappyPath",
			goldenPath: fmt.Sprintf("testdata/%s", "HappyPath"),
			args: args{
				sourcedir: "testdata/tarTestdir",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarContents, err := BuildTarFromDir(tt.args.sourcedir)
			if err != nil {
				t.Fatal(err)
			}
			gotContents, err := buildStringFromTar(gotTarContents)
			if err != nil {
				t.Error(err)
			}
			if tt.goldenPath != "" {
				if *update {
					err = ioutil.WriteFile(tt.goldenPath, []byte(gotContents), 0644)
					if err != nil {
						t.Fatalf("Error writing golden file %v", err)
					}
				}
				var aux []byte
				aux, err = ioutil.ReadFile(tt.goldenPath)
				if err != nil {
					t.Error(err)
				}
				tt.wantTarContents = string(aux)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("BuildTarFromDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantTarContents != gotContents {
				t.Errorf("Error want diffrent from got, want\n:%s\ngot:\n%s\n", tt.wantTarContents, gotContents)

			}
		})
	}
}

func buildStringFromTar(tarContents *bytes.Buffer) (string, error) {
	var err error
	st := make(map[string]string)
	var h *tar.Header
	t := tar.NewReader(tarContents)
	for {
		h, err = t.Next()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			break
		}
		buff := bytes.Buffer{}
		_, err = buff.ReadFrom(t)
		if err != nil {
			break
		}
		st[h.Name] = buff.String()
	}
	if err != nil {
		return "", err
	}
	var aux []byte
	aux, err = json.Marshal(st)
	if err != nil {
		return "", err
	}
	return string(aux), nil
}

func Test_parseGitLogLine(t *testing.T) {
	type args struct {
		gitLogOutput string
	}
	tests := []struct {
		name       string
		args       args
		wantCommit string
		wantErr    bool
	}{
		{
			name: "HappyPath",
			args: args{
				gitLogOutput: "137559c Fix error in  vulcan-is-exposed. (#5)",
			},
			wantCommit: "137559c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCommit, err := parseGitLogLine(tt.args.gitLogOutput)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitLogLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotCommit != tt.wantCommit {
				t.Errorf("parseGitLogLine() = %v, want %v", gotCommit, tt.wantCommit)
			}
		})
	}
}

func TestGetCurrentSDKVersion(t *testing.T) {
	tests := []struct {
		name    string
		sdkPath string
		want    string
		wantErr bool
	}{
		{
			name:    "HappyPath",
			sdkPath: "github.com/manelmontilla/toml",
			want:    "v0.3.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Cfg.SDKPath = tt.sdkPath
			got, err := GetCurrentSDKVersion()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCurrentSDKVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetCurrentSDKVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchRepositories(t *testing.T) {
	tests := []struct {
		name               string
		want               []string
		apiResponse        string
		apiResponseHeaders map[string]string
		wantErr            bool
	}{
		{
			name:        "HappyPath",
			want:        []string{"vulcan-checks/check1", "vulcan-checks/check2"},
			apiResponse: "{\"repositories\":[\"vulcan-checks/check1\",\"vulcan-checks/check2\"]}",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			s := buildFakeDockerRegistryWithHeaders(tt.apiResponse, tt.apiResponseHeaders)
			defer s.Close()
			config.Cfg.DockerAPIBaseURL = s.URL
			config.Cfg.DockerRegistryPwd = "user"
			config.Cfg.DockerRegistryUser = "pwd"

			got, err := FetchRepositories()
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchRepositories() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchRepositories() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getLatestTag(t *testing.T) {
	type args struct {
		tags []string
	}
	tests := []struct {
		name          string
		args          args
		wantLatestTag string
		wantFound     bool
	}{
		{
			name: "Return false when no integer tags passed",
			args: args{
				tags: []string{"dkdkdkd", "dsdds"},
			},
			wantLatestTag: "0",
			wantFound:     false,
		},
		{
			name: "Return false and 0 when empty array of tags",
			args: args{
				tags: []string{},
			},
			wantLatestTag: "0",
			wantFound:     false,
		},
		{
			name: "Return true and 7",
			args: args{
				tags: []string{"sddsdsd", "5", "7", "dsds", "3"},
			},
			wantLatestTag: "7",
			wantFound:     true,
		},
	}
	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			gotLatestTag, gotFound := GetLatestTag(tt.args.tags)
			if gotLatestTag != tt.wantLatestTag {
				t.Errorf("getLatestTag() gotLatestTag = %v, want %v", gotLatestTag, tt.wantLatestTag)
			}
			if gotFound != tt.wantFound {
				t.Errorf("getLatestTag() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func Test_readDockerOutput(t *testing.T) {
	l := log.New(os.Stdout, "Test_readDockerOutput", log.LstdFlags)
	type args struct {
		r      io.Reader
		logger *log.Logger
	}
	tests := []struct {
		name      string
		args      args
		wantLines []string
		wantErr   bool
	}{
		{
			name: "Happy Path",
			args: args{
				logger: l,
				r:      bytes.NewBufferString(""),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLines, err := readDockerOutput(tt.args.r, tt.args.logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("readDockerOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sort.Strings(gotLines)
			sort.Strings(tt.wantLines)
			d := cmp.Diff(gotLines, tt.wantLines)
			if d != "" {
				t.Errorf("readDockerOutput() = ! got. Diffs:\n %s", d)
			}
		})
	}
}

func TestGoBuildDir(t *testing.T) {
	type args struct {
		checkDir string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "HappyPath",
			args: args{
				checkDir: "testdata/dummygo",
			},
		},
		{
			name: "Not compiling go",
			args: args{
				checkDir: "testdata/badgo",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := GoBuildDir(tt.args.checkDir); (err != nil) != tt.wantErr {
				t.Errorf("GoBuildDir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
