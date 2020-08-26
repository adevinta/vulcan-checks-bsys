package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	vulcanreport "github.com/adevinta/vulcan-report"
)

// CheckState defines the fields of a check relevant during its execution.
type CheckState struct {
	Status   string              `json:"status,omitempty"`
	Progress float32             `json:"progress,omitempty"`
	Report   vulcanreport.Report `json:"report,omitempty"`
	Error    string              `json:"error,omitempty"`
}

// Check defines the relevant informartion for the runtime that executes and
// monitors checks.
type Check struct {
	ID    string
	State CheckState
}

type Queue interface {
	Dequeue() (*Check, error)
	Enqueue(Check) error
}

type SimpleMQClientServer struct {
	q    *SimpleMQ
	Addr string
}

func NewSimpleMQClientServer() (SimpleMQClientServer, error) {
	addr, err := freeLocalAddr()
	if err != nil {
		return SimpleMQClientServer{}, err
	}

	q := New(addr, "check")
	s := SimpleMQClientServer{
		q:    q,
		Addr: fmt.Sprintf("http://%s", addr),
	}
	return s, nil
}

func (s SimpleMQClientServer) Start() error {
	return s.q.ListenAndServe()

}

func (s SimpleMQClientServer) WaitStart(timeout time.Duration) error {
	d := time.NewTimer(timeout)
	for {
		select {
		case <-d.C:
			return errors.New("timeout waiting for the queue to start")
		default:
			c, err := net.Dial("tcp", strings.Replace(s.Addr, "http://", "", -1))
			if err == nil {
				c.Close()
				return nil
			}
		}

	}
}

func (s SimpleMQClientServer) Stop() error {
	return s.q.Stop()
}

func (s SimpleMQClientServer) Dequeue() (*Check, error) {
	m := s.q.Dequeue()
	if m.Payload == "" {
		return nil, nil
	}
	c := new(CheckState)
	err := json.Unmarshal([]byte(m.Payload), c)
	if err != nil {
		return nil, err
	}
	return &Check{
		ID:    m.ExternalID,
		State: *c,
	}, nil
}

func (s SimpleMQClientServer) Enqueue(c Check) error {
	payload, err := json.Marshal(c.State)
	if err != nil {
		return err
	}
	m := Message{
		ExternalID: c.ID,
		Payload:    string(payload),
	}
	s.q.Enqueue(m)
	return nil
}

func freeLocalAddr() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return "", err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return "", err
	}

	a := l.Addr().String()
	a = strings.Split(a, ":")[1]
	a = "localhost:" + a
	l.Close()
	return a, nil
}
