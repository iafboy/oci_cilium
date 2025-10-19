// Copyright 2022 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package oci

import (
	"fmt"
	//	"io"
	"net/http"

	"github.com/cilium/cilium/pkg/safeio"
)

const (
	metadataBaseURL string = "http://169.254.169.254/opc/v2/"
)

func newClient() (*http.Client, error) {
	c := http.Client{}
	return &c, nil

	// config := common.DefaultConfigProvider()
	// c, err := core.NewComputeManagementClientWithConfigurationProvider(config)
	// if err != nil {
	// 	return nil, err
	// }

	// return &c, nil
}

func getMetadata(client *http.Client, path string) (string, error) {
	url := metadataBaseURL + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer Oracle")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata service returned status code: %v", resp.StatusCode)
	}

	defer resp.Body.Close()
	value, err := safeio.ReadAllLimit(resp.Body, safeio.MB)
	if err != nil {
		return "", fmt.Errorf("unable to read response for OCI metadata %q: %w", path, err)
	}

	return string(value), nil
}

// GetInstanceMetadata returns required OCI metadata
func GetInstanceMetadata() (instanceID, instanceShape, availabilityDomain, vcnID string, err error) {
	client, err := newClient()
	if err != nil {
		return
	}

	instanceID, err = getMetadata(client, "instance/id")
	if err != nil {
		return
	}

	instanceShape, err = getMetadata(client, "instance/shape")
	if err != nil {
		return
	}

	availabilityDomain, err = getMetadata(client, "instance/availabilityDomain")
	if err != nil {
		return
	}

	// Try to get VCN ID from the primary VNIC's subnet
	// OCI metadata service doesn't directly expose VCN ID, but we can get it from the subnet
	subnetID, subnetErr := getMetadata(client, "vnics/0/subnetId")
	if subnetErr != nil {
		// If we can't get subnet ID from metadata, VCN ID must be provided via --oci-vcn-id flag
		// Return empty vcnID and let the operator configuration handle it
		vcnID = ""
		return
	}

	// Note: We cannot resolve VCN ID from subnet without full OCI authentication
	// The actual VCN ID MUST be specified via the --oci-vcn-id operator flag
	// We do NOT use compartmentID as it's a different resource type
	vcnID = ""

	// Log the discovered subnet for debugging
	if subnetID != "" {
		// Subnet ID is available: the operator will use --oci-vcn-id to specify the VCN
		// This is the recommended approach as it's explicit and validated
	}

	return
}
