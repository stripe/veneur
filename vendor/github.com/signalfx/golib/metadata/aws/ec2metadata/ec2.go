package ec2metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// will need to update this version from time to time
var identityURL = "http://169.254.169.254/2016-09-02/dynamic/instance-identity/document"

// EC2Metadata contains metadata about an ec2 instance
type EC2Metadata struct {
	AvailabilityZone string `json:"availabilityZone"`
	InstanceType     string `json:"instanceType"`
	InstanceID       string `json:"instanceId"`
	ImageID          string `json:"imageId"`
	AccountID        string `json:"accountId"`
	Region           string `json:"region"`
	Architecture     string `json:"architecture"`
	// In SFX the dimension/property key is "AWSUniqueId" this struct field is
	// named AWSUniqueID to satisfy golint
	AWSUniqueID string
}

// GetAWSUniqueID returns the AWSUniqueId from the collected EC2Metadata
// Please note that the dimension/property used in SFX is named "AWSUniqueId"
// and not AWSUniqueID.  This function is named in order to satisfy golint demands
func (e *EC2Metadata) GetAWSUniqueID() (string, error) {
	if e.InstanceID == "" {
		return "", errors.New("unable to build AWSUniqueId (missing InstanceId)")
	}
	if e.AccountID == "" {
		return "", errors.New("unable to build AWSUniqueId (missing AccountId)")
	}
	if e.Region == "" {
		return "", errors.New("unable to build AWSUniqueId (missing Region)")
	}

	return fmt.Sprintf("%s_%s_%s", e.InstanceID, e.Region, e.AccountID), nil
}

// ToStringMap returns the ec2 metadata as a string map
func (e *EC2Metadata) ToStringMap() map[string]string {
	metadata := map[string]string{
		"aws_availability_zone": e.AvailabilityZone,
		"aws_instance_type":     e.InstanceType,
		"aws_instance_id":       e.InstanceID,
		"aws_image_id":          e.ImageID,
		"aws_account_id":        e.AccountID,
		"aws_region":            e.Region,
		"aws_architecture":      e.Architecture,
		"AWSUniqueId":           e.AWSUniqueID,
	}

	return metadata
}

// Get returns a map with aws metadata including the AWSUniqueID
func Get() (*EC2Metadata, error) {
	return GetWithIdentityURL(identityURL)
}

// GetWithIdentityURL returns a map with aws metadata including AWSUniqueID
func GetWithIdentityURL(url string) (*EC2Metadata, error) {
	metadata, err := requestAWSInfo(url)
	if err != nil {
		return metadata, errors.New("not an aws box")
	}

	if id, err := metadata.GetAWSUniqueID(); err == nil {
		metadata.AWSUniqueID = id
	}
	return metadata, nil
}

// requestAWSInfo makes a request to the desired aws metadata url and decodes
// the response into an EC2Metadata struct
func requestAWSInfo(url string) (metadata *EC2Metadata, err error) {
	metadata = &EC2Metadata{}
	httpClient := &http.Client{Timeout: 200 * time.Millisecond}

	// make the request
	var res *http.Response
	if res, err = httpClient.Get(url); err == nil {
		err = json.NewDecoder(res.Body).Decode(metadata)
		_ = res.Body.Close()
	}

	return metadata, err
}
