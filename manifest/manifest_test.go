/*
Copyright 2019 Adevinta
*/

package manifest

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/manelmontilla/toml"
)

var update = flag.Bool("update", false, "update golden files")

func TestRead(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name           string
		args           args
		wantGoldenFile bool
		want           Data
		wantErr        bool
	}{
		{
			name:           "HappyPath",
			wantGoldenFile: true,
			args: args{
				path: "testdata/HappyPath/manifest.toml",
			},
		},
		{
			name:           "TwoAssetTypes",
			wantGoldenFile: true,
			args: args{
				path: "testdata/TwoAssetTypes/manifest.toml",
			},
		},
		{
			name:           "ErrorDescriptionNotProvided",
			wantGoldenFile: false,
			args: args{
				path: "testdata/ErrorDecriptionNotProvided/manifest.toml",
			},
			wantErr: true,
		},
		{
			name:           "ErrorMalFormedOptionsProvided",
			wantGoldenFile: false,
			args: args{
				path: "testdata/ErrorMalFormedOptionsProvided/manifest.toml",
			},
			wantErr: true,
		},
		{
			name:           "WebAddress",
			wantGoldenFile: true,
			args: args{
				path: "testdata/WebAddressAssetType/manifest.toml",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Read(tt.args.path)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				return
			}

			if tt.wantGoldenFile {
				goldenFilePath := fmt.Sprintf("testdata/%sGoldenFile.toml", tt.name)
				if *update {
					buff := bytes.Buffer{}
					bw := bufio.NewWriter(&buff)
					encoder := toml.NewEncoder(bw)
					err = encoder.Encode(&got)
					if err != nil {
						t.Error(err)
					}
					err = bw.Flush()
					if err != nil {
						t.Error(err)
						return
					}
					err = ioutil.WriteFile(goldenFilePath, buff.Bytes(), 0644)
					if err != nil {
						t.Fatalf("Error writing golden file %v", err)
					}
				}
				_, err = toml.DecodeFile(goldenFilePath, &tt.want)
				if err != nil {
					t.Error(err)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
