/*
Copyright 2019 Adevinta
*/

package main

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/kr/pretty"

	check "github.com/adevinta/vulcan-check-sdk"
	"github.com/adevinta/vulcan-check-sdk/agent"
	"github.com/adevinta/vulcan-check-sdk/config"
	"github.com/adevinta/vulcan-check-sdk/state"
	"github.com/adevinta/vulcan-check-sdk/tools"
	report "github.com/adevinta/vulcan-report"
)

func listen() (ln net.Listener, err error) {
	return net.Listen("tcp", ":8080")
}

func TestTestCheck(t *testing.T) {
	type args struct {
		opts   string
		target string
	}
	tests := []struct {
		name       string
		args       args
		wantResult state.State
		wantErr    error
	}{
		{
			name: "Happy path",
			args: args{opts: "", target: "127.0.0.1:8080"},
			wantResult: state.State{
				Progress: 1.0,
				Report: state.State{
					ResultData: report.ResultData{
						Score: 0.0,
					},
					CheckID:       "TEST_CHECK",
					ChecktypeName: "TEST_CHECK",
					Target:        "127.0.0.1:8080",
					Status:        agent.StatusFinished,
					Vulnerabilities: []report.Vulnerability{
						report.Vulnerability{
							Summary: "Address accepting connections",
						},
					},
				},
				Status: agent.StatusFinished,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := listen()
			if err != nil {
				t.Errorf("TestTestCheck() error %v", err)
			}
			a := tools.NewReporter(name)

			conf := &config.Config{
				Check: config.CheckConfig{
					CheckID:       name,
					Opts:          tt.args.opts,
					Target:        tt.args.target,
					CheckTypeName: name,
				},
				Log: config.LogConfig{
					LogFmt:   "text",
					LogLevel: "debug",
				},
				CommMode: "push",
			}

			conf.Push.AgentAddr = a.URL
			c := check.NewCheckFromHandlerWithConfig(name, conf, run)
			var last agent.State
			go func() {
				for msg := range a.Msgs {
					// We clear the Time fields by setting then to zero val
					// this is because comparing times with equality has no sense.
					msg.Report.StartTime = time.Time{}
					msg.Report.EndTime = time.Time{}
					last = msg
				}
			}()
			c.RunAndServe()
			a.Stop()
			_ = l.Close()
			if err != tt.wantErr {
				t.Errorf("TestTestCheck() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(last, tt.wantResult) {
				t.Errorf("TestTestCheck(), got : %s, want %s. Diffs: %s", pretty.Sprint(last), pretty.Sprint(tt.wantResult), pretty.Diff(last, tt.wantResult))
			}
		})
	}
}
