package main

import (
	"context"
	"net"
	"time"

	check "github.com/adevinta/vulcan-check-sdk"
	"github.com/adevinta/vulcan-check-sdk/state"
	report "github.com/adevinta/vulcan-report"
)

const name = "TEST_CHECK"

var (
	timeout         time.Duration = 3
	defaultProtocol               = "tcp"
	logger                        = check.NewCheckLog(name)
)

func run(ctx context.Context, target, assetType, optJSON string, state state.State) (err error) {

	addr := target
	d := &net.Dialer{Timeout: time.Second * timeout}
	_, err = d.DialContext(ctx, defaultProtocol, addr)
	logger.WithField("target", target).Debug("dialing")
	vuln := true
	// We should check if the call has been canceled or we have another error.
	if err != nil {
		if _, ok := err.(net.Error); ok {
			vuln = false
		} else {
			return err
		}
	}
	var title = "Address accepting connections"
	if vuln {
		state.AddVulnerabilities(report.Vulnerability{
			Summary: title,
			Score:   1.0,
		})
	}
	return nil
}

func main() {

	c := check.NewCheckFromHandler(name, run)
	c.RunAndServe()

}
