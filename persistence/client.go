/*
Copyright 2019 Adevinta
*/

package persistence

import (
	"fmt"
	"net/http"

	"gopkg.in/resty.v1"
)

var (
	checktypeBaseURL = "v1/checktypes"
)

// Checktype defines the data needed to publish a new Check.
type Checktype struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Timeout      int      `json:"timeout,omitempty"`
	Image        string   `json:"image"`
	Options      string   `json:"options,omitempty"`
	RequiredVars []string `json:"required_vars"`
	QueueName    string   `json:"queue_name,omitempty"`
	Assets       []string `json:"assets"`
}

type ChecktypeV2 struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Timeout      int                    `json:"timeout,omitempty"`
	Image        string                 `json:"image"`
	Options      map[string]interface{} `json:"options,omitempty"`
	RequiredVars []string               `json:"required_vars"`
	QueueName    string                 `json:"queue_name,omitempty"`
	Assets       []string               `json:"assets"`
}

type checkTypePostRequest struct {
	Check Checktype `json:"checktype"`
}

// Client used to interface with the persistence service.
type Client interface {
	PublishChecktype(Checktype) (*PublishChecktypeResult, error)
}

type client struct {
	client *resty.Client
}

// PublishCheckType publish a new check.
func (c *client) PublishChecktype(check Checktype) (*PublishChecktypeResult, error) {
	res := &PublishChecktypeResult{}

	p := c.client.R().SetBody(checkTypePostRequest{Check: check}).SetResult(&PublishChecktypeResultMsg{})
	r, err := p.Post(checktypeBaseURL)
	if err != nil {
		return res, err
	}
	if r.StatusCode() != int(http.StatusCreated) {
		return res, fmt.Errorf("Error posting new checktype to persistence service, status:%v", r.StatusCode())
	}
	aux := p.Result.(*PublishChecktypeResultMsg)
	return &aux.Checktype, nil
}

// CheckTypeLink handy struct for unmarshal the checktype create response from persistence.
type CheckTypeLink struct {
	Self string `json:"self"`
}

// PublishChecktypeResult information returned by the persistence service after a new checktype has added.
type PublishChecktypeResult struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Timeout      int           `json:"timeout"`
	Enabled      bool          `json:"enabled"`
	Options      interface{}   `json:"options"`
	RequiredVars []string      `json:"required_vars"`
	QueueName    string        `json:"queue_name,omitempty"`
	Image        string        `json:"image"`
	Links        CheckTypeLink `json:"links"`
	Assets       []string      `json:"assets"`
}

// PublishChecktypeResultMsg contains the data returned after a successful call to PublishChecktype
type PublishChecktypeResultMsg struct {
	Checktype PublishChecktypeResult `json:"checktype"`
}

// NewClient creates a new client for a given end point.
func NewClient(endPointURL string) Client {
	restyClient := resty.New()
	r := restyClient.SetHostURL(endPointURL)
	c := &client{client: r}
	return c
}
