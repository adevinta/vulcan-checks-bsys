/*
Copyright 2019 Adevinta
*/

package persistence

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type mockHandleRequest func(r *http.Request) (int, interface{})

func newHTTPServerMock(mHandler mockHandleRequest) (s *httptest.Server) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, resp := mHandler(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		e := json.NewEncoder(w)
		// The mock is designed to send messages that can be serialized to json,
		// so no error expected here. Still, if it happens, the only thing we can do is panic.
		err := e.Encode(resp)
		if err != nil {
			panic(err)
		}

	})
	return httptest.NewServer(h)
}

func Test_client_PublishChecktype(t *testing.T) {
	type args struct {
		check Checktype
	}
	tests := []struct {
		name        string
		args        args
		mockHandler mockHandleRequest
		want        PublishChecktypeResult
		wantErr     bool
	}{
		{
			name: "HappyPath",
			args: args{
				check: Checktype{
					Description:  "Description",
					Image:        "image",
					Name:         "Name",
					Timeout:      700,
					Options:      "{\"one\":\"two\"}",
					RequiredVars: []string{"TEST_ENV_VAR"},
					QueueName:    "NessusQueue",
					Assets:       []string{"DomainName"},
				},
			},
			want: PublishChecktypeResult{
				ID:           "OneID",
				Name:         "Name",
				Description:  "Description",
				Timeout:      700,
				Enabled:      true,
				Options:      "{\"one\":\"two\"}",
				RequiredVars: []string{"TEST_ENV_VAR"},
				QueueName:    "NessusQueue",
				Image:        "image",
				Links: CheckTypeLink{
					Self: "self",
				},
				Assets: []string{"DomainName"},
			},
			mockHandler: func(r *http.Request) (int, interface{}) {
				fmt.Printf("path: %s", r.URL.Path)
				if r.URL.Path != "/"+checktypeBaseURL {
					return http.StatusBadRequest, nil
				}
				d := json.NewDecoder(r.Body)
				req := &checkTypePostRequest{}
				err := d.Decode(req)
				if err != nil {
					return http.StatusBadRequest, nil
				}
				return http.StatusCreated, PublishChecktypeResultMsg{
					Checktype: PublishChecktypeResult{
						ID:           "OneID",
						Name:         req.Check.Name,
						Description:  req.Check.Description,
						Timeout:      req.Check.Timeout,
						Enabled:      true,
						Options:      req.Check.Options,
						RequiredVars: req.Check.RequiredVars,
						QueueName:    req.Check.QueueName,
						Image:        req.Check.Image,
						Links: CheckTypeLink{
							Self: "self",
						},
						Assets: []string{"DomainName"},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newHTTPServerMock(tt.mockHandler)
			c := NewClient(mock.URL)
			got, err := c.PublishChecktype(tt.args.check)
			mock.Close()
			if (err != nil) != tt.wantErr {
				t.Errorf("got error error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, &tt.want) {
				t.Errorf("got %v+, want %+v", got, tt.want)
			}
		})
	}
}
