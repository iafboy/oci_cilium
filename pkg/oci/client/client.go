// Copyright 2022 Authors of Cilium
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

package client

import (
	"context"
	"fmt"
	"time"

	"github.com/cilium/cilium/pkg/cidr"
	ipamTypes "github.com/cilium/cilium/pkg/ipam/types"
	"github.com/cilium/cilium/pkg/logging"
	"github.com/cilium/cilium/pkg/logging/logfields"
	"github.com/cilium/cilium/pkg/oci/types"
	vnicTypes "github.com/cilium/cilium/pkg/oci/vnic/types"

	ociCommon "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/resourcesearch"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

var log = logging.DefaultLogger.WithField(logfields.LogSubsys, "oci-client")

var maxAttachRetries = wait.Backoff{
	Duration: 4 * time.Second,
	Factor:   1,
	Jitter:   0.1,
	Steps:    4,
	Cap:      0,
}

type OCIClient struct {
	// Compartment:VCN = 1:N
	CompartmentID string

	// identityClient       *identity.IdentityClient
	VirtualNetworkClient *core.VirtualNetworkClient
	ComputeClient        *core.ComputeClient
	ResourceSearchClient *resourcesearch.ResourceSearchClient
}

func (c *OCIClient) GetInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	resp, err := c.ComputeClient.GetInstance(ctx, core.GetInstanceRequest{
		InstanceId: &instanceID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s: %w", instanceID, err)
	}

	instance := &types.Instance{
		Id:            *resp.Id,
		CompartmentId: *resp.CompartmentId,
	}
	if resp.DisplayName != nil {
		instance.DisplayName = *resp.DisplayName
	}

	return instance, nil
}

func (c *OCIClient) GetVnicAttachments(ctx context.Context, compartmentID string, instanceID *string) ([]types.VnicAttachment, error) {
	request := core.ListVnicAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    instanceID,
	}
	resp, err := c.ComputeClient.ListVnicAttachments(ctx, request)
	if err != nil {
		return nil, err
	}

	if resp.Items == nil || len(resp.Items) == 0 {
		return nil, nil
	}

	result := make([]types.VnicAttachment, 0, len(resp.Items))
	for _, item := range resp.Items {
		attachment := types.VnicAttachment{
			Id:          *item.Id,
			VnicId:      *item.VnicId,
			InstanceId:  *item.InstanceId,
			DisplayName: *item.DisplayName,
		}
		result = append(result, attachment)
	}

	return result, nil
}

func (c *OCIClient) GetVnic(ctx context.Context, vnicID string) (*types.Vnic, error) {
	resp, err := c.VirtualNetworkClient.GetVnic(ctx, core.GetVnicRequest{
		VnicId: &vnicID,
	})
	if err != nil {
		return nil, err
	}

	vnic := &types.Vnic{
		Id:        resp.Vnic.Id,
		SubnetId:  resp.Vnic.SubnetId,
		IsPrimary: resp.Vnic.IsPrimary,
		PrivateIp: resp.Vnic.PrivateIp,
	}

	return vnic, nil
}

func (c *OCIClient) ListPrivateIPs(ctx context.Context, vnicID string) ([]types.PrivateIP, error) {
	request := core.ListPrivateIpsRequest{
		VnicId: &vnicID,
	}
	resp, err := c.VirtualNetworkClient.ListPrivateIps(ctx, request)
	if err != nil {
		return nil, err
	}

	if resp.Items == nil || len(resp.Items) == 0 {
		return nil, nil
	}

	result := make([]types.PrivateIP, 0, len(resp.Items))
	for _, item := range resp.Items {
		if *item.IsPrimary {
			continue
		}
		privateIP := types.PrivateIP{
			Id:        item.Id,
			IpAddress: item.IpAddress,
		}
		result = append(result, privateIP)
	}

	return result, nil
}

func (c *OCIClient) ListVCNs(ctx context.Context) (ipamTypes.VirtualNetworkMap, error) {
	result := ipamTypes.VirtualNetworkMap{}

	var limit int = 100
	var page *string = nil
	for i := 1; ; i++ {
		request := core.ListVcnsRequest{
			CompartmentId: &c.CompartmentID,
			Limit:         &limit,
			Page:          page,
		}
		resp, err := c.VirtualNetworkClient.ListVcns(ctx, request)
		if err != nil {
			return result, err
		}

		if resp.Items == nil || len(resp.Items) == 0 {
			log.Info("Got empty VCN list from OCI")
			break
		}
		// log.Infof("ListVCNs successful: %v", resp.Items)

		for _, v := range resp.Items {
			cidrBlocks := v.CidrBlocks
			if len(cidrBlocks) < 1 {
				log.WithField("vcnID", *v.Id).Info("ListVCNs: skip VCN with empty CidrBlocks")
				continue
			}

			result[*v.Id] = &ipamTypes.VirtualNetwork{
				ID: *v.Id,
				// PrimaryCIDR: *v.CidrBlock,
				// CIDRs: v.CidrBlocks, // no this field in current sdk

				PrimaryCIDR: cidrBlocks[0],
				CIDRs:       cidrBlocks[1:],
			}
		}

		if resp.OpcNextPage != nil {
			page = resp.OpcNextPage
			continue
		}

		break
	}

	// log.Infof("Final VirtualNetworkMap: ")
	// for _, v := range result {
	// 	log.Infof("VCN info: %v", v)
	// }

	return result, nil
}

func getAvailableIpAddressCount(c *cidr.CIDR) int {
	if c == nil {
		return 0
	}

	// https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/managingVCNs_topic-Overview_of_VCNs_and_Subnets.htm
	n := c.AvailableIPs() - 3
	if n < 0 {
		n = 0
	}

	return n
}

func (c *OCIClient) ListSubnets(ctx context.Context) (ipamTypes.SubnetMap, error) {
	subnets := ipamTypes.SubnetMap{}

	var limit int = 100
	var page *string = nil
	for i := 1; ; i++ {
		request := core.ListSubnetsRequest{
			CompartmentId: &c.CompartmentID,
			Limit:         &limit,
			Page:          page,
		}

		resp, err := c.VirtualNetworkClient.ListSubnets(ctx, request)
		if err != nil {
			return subnets, err
		}

		if resp.Items == nil || len(resp.Items) == 0 {
			log.Info("Got empty subnet list from OCI")
			break
		}

		for _, s := range resp.Items {
			subnetCIDR, err := cidr.ParseCIDR(*s.CidrBlock)
			if err != nil {
				continue
			}

			subnet := &ipamTypes.Subnet{
				ID:                 *s.Id,
				CIDR:               subnetCIDR,
				AvailableAddresses: getAvailableIpAddressCount(subnetCIDR),
				Tags:               s.FreeformTags,
			}

			if s.DisplayName != nil {
				subnet.Name = *s.DisplayName
			}
			if s.VcnId != nil {
				subnet.VirtualNetworkID = *s.VcnId
			}
			// This attribute will be null if this is a regional subnet instead
			// of an AD-specific subnet. Oracle recommends creating regional subnets.
			if s.AvailabilityDomain != nil {
				subnet.AvailabilityZone = *s.AvailabilityDomain
			}

			subnets[*s.Id] = subnet
		}

		if resp.OpcNextPage != nil {
			page = resp.OpcNextPage
			continue
		}

		break
	}

	// log.Infof("Final SubnetMap: ")
	// for _, v := range subnets {
	// 	log.Infof("Subnets info: %v", v)
	// }

	return subnets, nil
}

// GetInstanceMaxVnicAttachments 返回实例级 shape-config 的 max VNIC attachments
func (c *OCIClient) GetInstanceMaxVnicAttachments(ctx context.Context, instanceID string) (int, error) {
	req := core.GetInstanceRequest{
		InstanceId: &instanceID,
	}
	resp, err := c.ComputeClient.GetInstance(ctx, req)
	if err != nil {
		return 0, err
	}
	if resp.Instance.ShapeConfig == nil || resp.Instance.ShapeConfig.MaxVnicAttachments == nil {
		// 没有取到，交由调用方使用原 limits 值
		return 0, nil
	}
	max := int(*resp.Instance.ShapeConfig.MaxVnicAttachments)
	log.WithFields(logrus.Fields{
		"instanceID":           instanceID,
		"adaptersFromInstance": max,
	}).Info("GetInstanceMaxVnicAttachments: resolved")
	return max, nil
}

// NOTE: use search instead of ListInstances() may speedup:
// query instance resources where lifeCycleState = 'RUNNING' && compartmentId = '<id>'
func (c *OCIClient) ListInstances(ctx context.Context, vcns ipamTypes.VirtualNetworkMap, subnets ipamTypes.SubnetMap) (*ipamTypes.InstanceMap, error) {
	instanceMap := ipamTypes.NewInstanceMap()

	request := core.ListInstancesRequest{
		CompartmentId: &c.CompartmentID,
	}
	// TODO: use pagination
	resp, err := c.ComputeClient.ListInstances(ctx, request)
	if err != nil {
		return nil, err
	}

	if resp.Items == nil || len(resp.Items) == 0 {
		log.Info("Get empty instance list from OCI, returning empty InstanceMap")
		return instanceMap, nil
	}

	// NOTE: workaround with ListVnicAttachments as OCI doesn't provide ListENI api
	for _, inst := range resp.Items {
		if inst.LifecycleState != core.InstanceLifecycleStateRunning {
			log.WithFields(logrus.Fields{
				// "instanceID":   *inst.Id,
				"instanceName": *inst.DisplayName,
				"state":        inst.LifecycleState,
			}).Debug("Skip non-running state OCI instance")

			continue
		}

		request := core.ListVnicAttachmentsRequest{
			CompartmentId: &c.CompartmentID,
			InstanceId:    inst.Id,
		}
		resp, err := c.ComputeClient.ListVnicAttachments(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("ListVnicAttachments failed: %v", err)
		}

		if resp.Items == nil || len(resp.Items) == 0 {
			log.Warn("Get empty vnic attachment list from OCI")
			continue
		}

		for _, va := range resp.Items {
			// 仅统计 ATTACHED 的 VNIC，避免 DETACHING/DETACHED 干扰
			if va.LifecycleState != core.VnicAttachmentLifecycleStateAttached {
				continue
			}
			if va.VnicId == nil {
				// VNICs still in attaching process, just skip it
				log.WithFields(logrus.Fields{
					"vnicID":           "nil",
					"vnicAttachmentID": va.Id,
					"instanceID":       inst.Id,
				}).Info("Skip VnicAttachment")
				continue
			}
			vnicID := *va.VnicId

			// Get VNIC details
			request := core.GetVnicRequest{
				VnicId: ociCommon.String(vnicID),
			}
			resp, err := c.VirtualNetworkClient.GetVnic(ctx, request)
			if err != nil {
				serviceErr, ok := ociCommon.IsServiceError(err)
				if !ok {
					return nil, fmt.Errorf("GetVnic failed: %v", err)
				}

				if serviceErr.GetHTTPStatusCode() == 404 {
					log.WithFields(logrus.Fields{
						"vnicID":           *va.VnicId,
						"vnicAttachmentID": *va.Id,
						// "instanceID":       *inst.Id,
						"instanceName": *inst.DisplayName,
					}).Info("GetVnic returned 404")
					continue
				}

				return nil, fmt.Errorf("GetVnic failed: %v", err)
			}

			// Get private IP addresses on this VNIC
			privateIPs := []string{}
			{
				req2 := core.ListPrivateIpsRequest{
					VnicId: ociCommon.String(vnicID),
				}
				r2, err := c.VirtualNetworkClient.ListPrivateIps(ctx, req2)
				if err != nil {
					return nil, fmt.Errorf("ListPrivateIps failed: %v", err)
				}

				if r2.Items == nil {
					log.Warn("Get empty private IP list for VNIC from OCI")
					continue
				}

				for _, ip := range r2.Items {
					if *ip.IsPrimary {
						continue
					}

					privateIPs = append(privateIPs, *ip.IpAddress)
				}
			}

			// VNIC information sum up
			respVnic := resp.Vnic
			vnic := &vnicTypes.VNIC{
				ID:        vnicID,
				MAC:       *respVnic.MacAddress,
				PrimaryIP: *respVnic.PrivateIp,
				IsPrimary: *respVnic.IsPrimary,
				Addresses: privateIPs,
			}

			if va.DisplayName != nil {
				vnic.Description = *respVnic.DisplayName
			}
			if va.AvailabilityDomain != nil {
				vnic.AvailabilityDomain = *respVnic.AvailabilityDomain
			}

			vcnID := ""
			subnetID := *respVnic.SubnetId
			if subnets != nil {
				if net, ok := subnets[subnetID]; ok {
					vnic.Subnet.ID = subnetID
					vnic.Subnet.CIDR = net.CIDR.String()
					vcnID = net.VirtualNetworkID
				}
			}

			if vcnID != "" && vcns != nil {
				if vcn, ok := vcns[vcnID]; ok {
					vnic.VCN.ID = vcnID
					vnic.VCN.CidrBlocks = []string{vcn.PrimaryCIDR}
					vnic.VCN.CidrBlocks = append(vnic.VCN.CidrBlocks, vcn.CIDRs...)
				}
			}

			if *inst.DisplayName == "m_node3" {
				log.WithFields(logrus.Fields{
					"instanceName": *inst.DisplayName,
					"parsedVnic":   vnic,
				}).Info("ListInstances")
			}

			instanceMap.Update(*inst.Id, ipamTypes.InterfaceRevision{
				Resource: vnic,
			})
		}
	}

	// log.Infof("Final instance map: %v", *instanceMap)
	return instanceMap, nil
}

func (c *OCIClient) GetVPC(ctx context.Context, vpcID string) (*ipamTypes.VirtualNetwork, error) {
	panic("GetVPC not implemented by OCI")
	return nil, nil
}
func (c *OCIClient) GetSecurityGroups(ctx context.Context) (types.SecurityGroupMap, error) {
	return nil, nil
}

func (c *OCIClient) AttachNetworkInterface(ctx context.Context, instanceID, subnetID string) (string, error) {
	resp, err := c.ComputeClient.AttachVnic(
		ctx,
		core.AttachVnicRequest{
			AttachVnicDetails: core.AttachVnicDetails{
				CreateVnicDetails: &core.CreateVnicDetails{
					SubnetId: &subnetID,
					// AssignPublicIp: common.Bool(false),
				},
				InstanceId: &instanceID,
			},
		})
	if err != nil {
		return "", err
	}

	return *resp.Id, nil
}

func (c *OCIClient) WaitVNICAttached(ctx context.Context, vnicAttachmentID string) (*vnicTypes.VNIC, error) {
	// instanceID := ""
	vnicID := ""

	if err := wait.ExponentialBackoffWithContext(ctx, maxAttachRetries, func(ctx context.Context) (done bool, err error) {
		resp, err := c.ComputeClient.GetVnicAttachment(ctx, core.GetVnicAttachmentRequest{
			VnicAttachmentId: &vnicAttachmentID,
		})
		if err != nil {
			return false, err
		}

		if resp.VnicAttachment.LifecycleState == core.VnicAttachmentLifecycleStateAttached {
			// instanceID = *resp.VnicAttachment.InstanceId
			vnicID = *resp.VnicAttachment.VnicId
			return true, nil
		}

		return false, nil
	}); err != nil {
		return nil, err
	}

	// Parse to IPAM VNIC presentation
	request := core.GetVnicRequest{
		VnicId: &vnicID,
	}
	resp, err := c.VirtualNetworkClient.GetVnic(ctx, request)
	if err != nil {
		return nil, err
	}
	respVnic := resp.Vnic

	vnic := vnicTypes.VNIC{
		ID:        vnicID,
		MAC:       *respVnic.MacAddress,
		PrimaryIP: *respVnic.PrivateIp,
		IsPrimary: *respVnic.IsPrimary,
		// TODO: Addresses: get from OCI
	}

	if respVnic.DisplayName != nil {
		vnic.Description = *respVnic.DisplayName
	}
	if respVnic.AvailabilityDomain != nil {
		vnic.AvailabilityDomain = *respVnic.AvailabilityDomain
	}

	return &vnic, nil
}
func (c *OCIClient) DeleteNetworkInterface(ctx context.Context, vnicID string) error {
	return nil
}

// Allocate an IP address for the given VNIC
func (c *OCIClient) AssignPrivateIPAddresses(ctx context.Context, vnicID string, toAllocate int) ([]string, error) {
	ips := []string{}

	request := core.CreatePrivateIpRequest{
		CreatePrivateIpDetails: core.CreatePrivateIpDetails{
			VnicId: &vnicID,
		},
	}
	resp, err := c.VirtualNetworkClient.CreatePrivateIp(ctx, request)
	if err != nil {
		return ips, err
	}

	privateIP := resp.PrivateIp
	if privateIP.IpAddress == nil {
		return ips, fmt.Errorf("returned empty PrivateIp from OCI")
	}

	ipAddr := *privateIP.IpAddress

	if privateIP.IsPrimary == nil || *privateIP.IsPrimary == true {
		return ips, fmt.Errorf("returned PrivateIp from OCI is primary IP")
	}

	log.WithFields(logrus.Fields{
		"vnicID": vnicID,
		"ip":     ipAddr,
	}).Infof("Assign private IP to VNIC successful")

	ips = append(ips, ipAddr)
	return ips, nil
}

func (c *OCIClient) UnassignPrivateIPAddresses(ctx context.Context, vnicID string, addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}

	// First, we need to get the private IP IDs from the VNIC
	privateIPList, err := c.ListPrivateIPs(ctx, vnicID)
	if err != nil {
		return fmt.Errorf("failed to list private IPs for VNIC %s: %w", vnicID, err)
	}

	// Build a map of IP address to private IP ID
	ipToIDMap := make(map[string]string)
	for _, privateIP := range privateIPList {
		if privateIP.IpAddress != nil && privateIP.Id != nil {
			ipToIDMap[*privateIP.IpAddress] = *privateIP.Id
		}
	}

	// Delete each private IP
	var firstErr error
	successCount := 0
	for _, ipAddr := range addresses {
		privateIPID, ok := ipToIDMap[ipAddr]
		if !ok {
			log.WithFields(logrus.Fields{
				"vnicID": vnicID,
				"ip":     ipAddr,
			}).Warning("Private IP not found on VNIC, skipping deletion")
			continue
		}

		request := core.DeletePrivateIpRequest{
			PrivateIpId: ociCommon.String(privateIPID),
		}

		_, err := c.VirtualNetworkClient.DeletePrivateIp(ctx, request)
		if err != nil {
			log.WithFields(logrus.Fields{
				"vnicID":      vnicID,
				"ip":          ipAddr,
				"privateIPID": privateIPID,
			}).WithError(err).Error("Failed to delete private IP")
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successCount++
			log.WithFields(logrus.Fields{
				"vnicID":      vnicID,
				"ip":          ipAddr,
				"privateIPID": privateIPID,
			}).Info("Successfully deleted private IP")
		}
	}

	if firstErr != nil {
		return fmt.Errorf("failed to delete %d/%d private IPs: %w", len(addresses)-successCount, len(addresses), firstErr)
	}

	log.WithFields(logrus.Fields{
		"vnicID":       vnicID,
		"deletedCount": successCount,
	}).Info("Successfully unassigned private IP addresses")

	return nil
}
