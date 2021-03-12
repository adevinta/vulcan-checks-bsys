/*
Copyright 2021 Adevinta
*/

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/adevinta/vulcan-checks-bsys/manifest"
)

func TestMainFParam(t *testing.T) {
	testDataPath := "testdata"
	type mainArgs struct {
		checkDir string
	}
	type checkExecutionResult func() (bool, error)
	tests := []struct {
		large           bool
		name            string
		args            mainArgs
		checkExecResult checkExecutionResult
	}{
		{
			large: true,
			name:  "f param Happy path",
			args:  mainArgs{checkDir: path.Join(testDataPath, "testcheck")},
			checkExecResult: func() (ok bool, err error) {
				return true, nil
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := forceBuild(tt.args.checkDir)
			if err != nil {
				t.Errorf("Error checking test result %s, error: %v", tt.name, err)
			}
			res, err := tt.checkExecResult()
			if err != nil || !res {
				t.Errorf("Error checking test result %s, error: %v,result: %v", tt.name, err, res)
			}
		})
	}
}

func writeJSONResponse(w http.ResponseWriter, code int, r string, headers map[string]string) {
	for v, k := range headers {
		w.Header().Set(v, k)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, "%s", r)
}

func buildFakePersistence(result string, status int) *httptest.Server {
	return buildFakePersistenceWithHeaders(result, nil, status)
}

func buildFakePersistenceWithHeaders(result string, headers map[string]string, status int) *httptest.Server {
	handle := func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, status, result, headers)
	}
	return httptest.NewServer(http.HandlerFunc(handle))
}

func Test_pubChecktypeToPersistence(t *testing.T) {
	type args struct {
		checkName string
		metadata  manifest.Data
		imagePath string
		fail      bool
	}
	tests := []struct {
		name              string
		args              args
		apiResponse       string
		persistenceStatus int
		wantErr           bool
	}{
		{
			name: "HappyPathFail",
			args: args{
				checkName: "check",
				imagePath: "docker.example.com/check:2",
				metadata:  manifest.Data{},
				fail:      true,
			},
			apiResponse:       "{\"properties\":{\"docker.label.commit\": [\"01234a\"],\"docker.label.sdk-version\": [\"8e938a5\"]}}",
			persistenceStatus: http.StatusCreated,
		},
		{
			name: "HappyPathNoFail",
			args: args{
				checkName: "check",
				imagePath: "docker.example.com/check:2",
				metadata:  manifest.Data{},
				fail:      false,
			},
			apiResponse:       "{\"properties\":{\"docker.label.commit\": [\"01234a\"],\"docker.label.sdk-version\": [\"8e938a5\"]}}",
			persistenceStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			s := buildFakePersistence(tt.apiResponse, tt.persistenceStatus)
			defer s.Close()
			if err := pubChecktypeToPersistence(tt.args.checkName, tt.args.metadata, tt.args.imagePath, tt.args.fail, s.URL); (err != nil) != tt.wantErr {
				t.Errorf("pubChecktypeToPersistence() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
