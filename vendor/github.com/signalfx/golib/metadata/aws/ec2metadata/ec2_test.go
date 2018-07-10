package ec2metadata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// test fixture to mock aws identity document server
type requestHandler struct {
	response string
}

func (rh *requestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, rh.response)
}

func TestAWSMetadata_Get(t *testing.T) {
	tests := []struct {
		name     string
		identity map[string]interface{}
		want     map[string]string
		wantErr  bool
	}{
		{
			name: "Test successful connection",
			identity: map[string]interface{}{
				"availabilityZone": "testAZ",
				"instanceType":     "testInstanceType",
				"instanceId":       "testInstanceId",
				"imageId":          "testImageId",
				"accountId":        "testAccountId",
				"region":           "testRegion",
				"architecture":     "testArchitecture",
			},
			want: map[string]string{
				"aws_availability_zone": "testAZ",
				"aws_instance_type":     "testInstanceType",
				"aws_instance_id":       "testInstanceId",
				"aws_image_id":          "testImageId",
				"aws_account_id":        "testAccountId",
				"aws_region":            "testRegion",
				"aws_architecture":      "testArchitecture",
				"AWSUniqueId":           "testInstanceId_testRegion_testAccountId",
			},
			wantErr: false,
		},
		{
			name: "Test successful connection but missing instanceID",
			identity: map[string]interface{}{
				"availabilityZone": "testAZ",
				"instanceType":     "testInstanceType",
				"imageId":          "testImageId",
				"accountId":        "testAccountId",
				"region":           "testRegion",
				"architecture":     "testArchitecture",
			},
			want: map[string]string{
				"aws_availability_zone": "testAZ",
				"aws_instance_type":     "testInstanceType",
				"aws_instance_id":       "",
				"aws_image_id":          "testImageId",
				"aws_account_id":        "testAccountId",
				"aws_region":            "testRegion",
				"aws_architecture":      "testArchitecture",
				"AWSUniqueId":           "",
			},
			wantErr: false,
		},
		{
			name: "Test successful connection but missing account id",
			identity: map[string]interface{}{
				"availabilityZone": "testAZ",
				"instanceType":     "testInstanceType",
				"imageId":          "testImageId",
				"instanceId":       "testInstanceId",
				"accountId":        "",
				"region":           "testRegion",
				"architecture":     "testArchitecture",
			},
			want: map[string]string{
				"aws_availability_zone": "testAZ",
				"aws_instance_type":     "testInstanceType",
				"aws_instance_id":       "testInstanceId",
				"aws_image_id":          "testImageId",
				"aws_account_id":        "",
				"aws_region":            "testRegion",
				"aws_architecture":      "testArchitecture",
				"AWSUniqueId":           "",
			},
			wantErr: false,
		},
		{
			name: "Test successful connection but missing region",
			identity: map[string]interface{}{
				"availabilityZone": "testAZ",
				"instanceType":     "testInstanceType",
				"imageId":          "testImageId",
				"instanceId":       "testInstanceId",
				"accountId":        "testAccountId",
				"region":           "",
				"architecture":     "testArchitecture",
			},
			want: map[string]string{
				"aws_availability_zone": "testAZ",
				"aws_instance_type":     "testInstanceType",
				"aws_instance_id":       "testInstanceId",
				"aws_image_id":          "testImageId",
				"aws_account_id":        "testAccountId",
				"aws_region":            "",
				"aws_architecture":      "testArchitecture",
				"AWSUniqueId":           "",
			},
			wantErr: false,
		},
		{
			name:     "Test unsuccessful connection",
			identity: map[string]interface{}{},
			want: map[string]string{
				"aws_availability_zone": "",
				"aws_instance_type":     "",
				"aws_instance_id":       "",
				"aws_image_id":          "",
				"aws_account_id":        "",
				"aws_region":            "",
				"aws_architecture":      "",
				"AWSUniqueId":           "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.wantErr {
				// marshall identity into json
				js, _ := json.Marshal(tt.identity)

				// set up mock server
				handler := &requestHandler{response: string(js)}
				server := httptest.NewServer(handler)
				defer server.Close()

				// set the identityURL to point to mock server
				identityURL = server.URL
			} else {
				identityURL = "NotARealDomainName"
			}
			// create new aws metadata
			awsMeta, err := Get()
			if (err != nil) != tt.wantErr {
				t.Errorf("EC2Metadata.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := awsMeta.ToStringMap()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EC2Metadata.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
