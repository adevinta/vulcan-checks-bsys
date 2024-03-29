/*
Copyright 2019 Adevinta
*/

package manifest

import (
	"encoding/json"
	"errors"

	"fmt"

	"github.com/manelmontilla/toml"
)

// AssetType defines the valid types of assets a check can accept.
type AssetType int

const (
	// IP represents an IP assettype.
	IP AssetType = iota
	// Hostname represents a hostname assettype.
	Hostname
	// DomainName represents an domain name assettype.
	DomainName
	// AWSAccount represents an AWS account assettype.
	AWSAccount
	// IPRange represents an IP range assettype.
	IPRange
	// DockerImage represents a DockerImage asset type.
	DockerImage
	// WebAddress represents a WebAddress asset type.
	WebAddress
	// GitRepository represents a git repo asset type.
	GitRepository
	// GCPProject represents a GCP Project asset type.
	GCPProject
)

var assetTypeStrings = map[AssetType]string{
	IP:            "IP",
	Hostname:      "Hostname",
	DomainName:    "DomainName",
	AWSAccount:    "AWSAccount",
	IPRange:       "IPRange",
	DockerImage:   "DockerImage",
	WebAddress:    "WebAddress",
	GitRepository: "GitRepository",
	GCPProject:    "GCPProject",
}

// MarshalText returns string representation of a AssetType instance.
func (a *AssetType) MarshalText() (text []byte, err error) {
	s, err := a.String()
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// UnmarshalText creates a AssetType from its string representation.
func (a *AssetType) UnmarshalText(text []byte) error {
	val := string(text)
	for k, v := range assetTypeStrings {
		if v == val {
			*a = k
			return nil
		}
	}
	return fmt.Errorf("Error value %s is not a valid AssetType value", val)
}

func (a *AssetType) String() (string, error) {
	if _, ok := assetTypeStrings[*a]; !ok {
		return "", fmt.Errorf("value: %d is not a valid string representation of AssetType", a)
	}
	return assetTypeStrings[*a], nil
}

// AssetTypes represents and array of asset types supported by a concrete
// checktype.
type AssetTypes []*AssetType

// Strings converts a slice of Assettypes into a slice of strings.
func (a AssetTypes) Strings() ([]string, error) {
	res := []string{}
	for _, s := range a {
		txt, err := s.String()
		if err != nil {
			return nil, err
		}
		res = append(res, txt)
	}
	return res, nil
}

// Data contains all the data defined in the manifest.
type Data struct {
	Description  string
	Timeout      int
	Options      string
	RequiredVars []string
	QueueName    string
	AssetTypes   AssetTypes
}

// Read reads a manifest file.
func Read(path string) (Data, error) {
	d := Data{}
	m, err := toml.DecodeFile(path, &d)
	if err != nil {
		return d, err
	}
	if !m.IsDefined("Description") {
		return d, errors.New("Description field is mandatory")
	}

	if m.IsDefined("Options") {
		dummy := make(map[string]interface{})
		err = json.Unmarshal([]byte(d.Options), &dummy)
		if err != nil {
			err = fmt.Errorf("Error reading manifest file, Options field is not a valid json: %v", err)
			return d, err
		}
	}
	return d, nil
}
