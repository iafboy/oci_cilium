// Copyright 2021 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	ipamTypes "github.com/cilium/cilium/pkg/ipam/types"
)

// Instance represents an OCI compute instance
type Instance struct {
	// Id is the instance ID
	Id string

	// CompartmentId is the compartment ID where the instance resides
	CompartmentId string

	// DisplayName is the display name of the instance
	DisplayName string
}

// VnicAttachment represents an OCI VNIC attachment
type VnicAttachment struct {
	// Id is the VNIC attachment ID
	Id string

	// VnicId is the ID of the attached VNIC
	VnicId string

	// InstanceId is the ID of the instance this VNIC is attached to
	InstanceId string

	// DisplayName is the display name of the VNIC attachment
	DisplayName string
}

// Vnic represents an OCI VNIC
type Vnic struct {
	// Id is the VNIC ID
	Id *string

	// IsPrimary indicates if this is the primary VNIC
	IsPrimary *bool

	// SubnetId is the ID of the subnet this VNIC is in
	SubnetId *string

	// PrivateIp is the primary private IP address of the VNIC
	PrivateIp *string
}

// PrivateIP represents an OCI private IP
type PrivateIP struct {
	// Id is the private IP ID
	Id *string

	// IpAddress is the private IP address
	IpAddress *string
}

// SecurityGroup is the representation of an OCI Security Group
//
// +k8s:deepcopy-gen=true
type SecurityGroup struct {
	// ID is the SecurityGroup ID
	ID string

	// VPCID is the VPC ID in which the security group resides
	VPCID string

	// Tags are the tags of the security group
	Tags ipamTypes.Tags
}

// SecurityGroupMap indexes Security Groups by security group ID
type SecurityGroupMap map[string]*SecurityGroup
