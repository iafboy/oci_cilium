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

package vnic

import (
	"context"
	"fmt"
	"sort"

	"github.com/cilium/cilium/pkg/defaults"
	"github.com/cilium/cilium/pkg/ipam"
	"github.com/cilium/cilium/pkg/ipam/stats"
	"github.com/cilium/cilium/pkg/ipam/types"
	ipamTypes "github.com/cilium/cilium/pkg/ipam/types"
	v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/lock"
	"github.com/cilium/cilium/pkg/math"

	// "github.com/cilium/cilium/pkg/oci/utils"
	"github.com/cilium/cilium/pkg/oci/vnic/limits"
	vnicTypes "github.com/cilium/cilium/pkg/oci/vnic/types"

	"github.com/sirupsen/logrus"
)

// The following error constants represent the error conditions for
// CreateInterface without additional context embedded in order to make them
// usable for metrics accounting purposes.
const (
	errUnableToDetermineLimits   = "unable to determine limits"
	errUnableToGetSecurityGroups = "unable to get security groups"
	errUnableToCreateVNIC        = "unable to create VNIC"
	errUnableToAttachVNIC        = "unable to attach VNIC"
	errUnableToFindSubnet        = "unable to find matching subnet"
)

const (
	maxVNICIPCreate = 32
	maxVNICPerNode  = 24
)

type Node struct {
	// node contains the general purpose fields of a node
	node *ipam.Node

	// mutex protects members below this field
	mutex lock.RWMutex

	// vnics is the list of VNICs attached to the node indexed by VNIC ID.
	// Protected by Node.mutex.
	vnics map[string]vnicTypes.VNIC

	// k8sObj is the CiliumNode custom resource representing the node
	k8sObj *v2.CiliumNode

	// manager is the ecs node manager responsible for this node
	manager *InstancesManager

	// instanceID of the node
	instanceID string
}

// UpdatedNode is called when an update to the CiliumNode is received.
func (n *Node) UpdatedNode(obj *v2.CiliumNode) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.k8sObj = obj
}

// PopulateStatusFields fills in the status field of the CiliumNode custom
// resource with OCI specific information
func (n *Node) PopulateStatusFields(resource *v2.CiliumNode) {
	resource.Status.OCI.VNICs = map[string]vnicTypes.VNIC{}

	n.manager.ForeachInstance(
		n.node.InstanceID(),
		func(instanceID, interfaceID string, rev ipamTypes.InterfaceRevision) error {
			v, ok := rev.Resource.(*vnicTypes.VNIC)
			if ok {
				resource.Status.OCI.VNICs[interfaceID] = *v.DeepCopy()
			}
			return nil
		})

	return
}

// CreateInterface creates an additional interface with the instance and
// attaches it to the instance as specified by the CiliumNode. neededAddresses
// of secondary IPs are assigned to the interface up to the maximum number of
// addresses as allowed by the instance.
func (n *Node) CreateInterface(ctx context.Context, allocation *ipam.AllocationAction, scopedLog *logrus.Entry) (int, string, error) {
	log.WithFields(logrus.Fields{
		"node": n.instanceID,
	}).Info("CreateInterface for instance")

	l, limitsAvailable := n.getLimits()
	if !limitsAvailable {
		return 0, errUnableToDetermineLimits, fmt.Errorf(errUnableToDetermineLimits)
	}

	n.mutex.RLock()
	resource := *n.k8sObj
	n.mutex.RUnlock()

	// Must allocate secondary VNIC IPs as needed, up to VNIC instance limit
	toAllocate := math.IntMin(allocation.MaxIPsToAllocate, l.IPv4)
	toAllocate = math.IntMin(maxVNICIPCreate, toAllocate) // in first alloc no more than 10
	// Validate whether request has already been fulfilled in the meantime
	if toAllocate == 0 {
		log.Errorf("toAllocate == 0")
		return 0, "", nil
	}

	ociSpec := resource.Spec.OCI

	// If VCNID is not specified in the node spec, try to determine it from existing VNICs
	vcnID := ociSpec.VCNID
	if vcnID == "" {
		// Get VCN ID from the primary VNIC
		n.mutex.RLock()
		for _, vnic := range n.vnics {
			if vnic.IsPrimary {
				vcnID = vnic.VCN.ID
				scopedLog.WithField("vcnID", vcnID).Info("Using VCN ID from primary VNIC")
				break
			}
		}
		n.mutex.RUnlock()

		// If still empty after detection, return error
		if vcnID == "" {
			return 0,
				errUnableToFindSubnet,
				fmt.Errorf("VCN ID not specified in spec and unable to detect from primary VNIC")
		}
	}

	scopedLog.WithField("vcnID", vcnID).Info("Finding best subnet for VNIC allocation")
	bestSubnet := n.manager.FindSubnet(vcnID, ociSpec.AvailabilityDomain, toAllocate, ociSpec.SubnetTags)
	if bestSubnet == nil {
		return 0,
			errUnableToFindSubnet,
			fmt.Errorf(
				"no matching subnet available for interface creation (VCN=%s AZ=%s SubnetTags=%s)",
				vcnID,
				ociSpec.AvailabilityDomain,
				ociSpec.SubnetTags,
			)
	}

	// securityGroupIDs, err := n.getSecurityGroupIDs(ctx, ociSpec)
	// if err != nil {
	// 	return 0,
	// 		errUnableToGetSecurityGroups,
	// 		fmt.Errorf("%s %s", errUnableToGetSecurityGroups, err)
	// }

	scopedLog = scopedLog.WithFields(logrus.Fields{
		// "securityGroupIDs": securityGroupIDs,
		"bestSubnetID": bestSubnet.ID,
		"toAllocate":   toAllocate,
	})
	scopedLog.Info("No more IPs available, creating+attaching new VNIC")

	instanceID := n.node.InstanceID()
	n.mutex.Lock()
	defer n.mutex.Unlock()

	attachmentID, err := n.manager.api.AttachNetworkInterface(ctx, instanceID, bestSubnet.ID)
	if err != nil {
		return 0, errUnableToAttachVNIC, fmt.Errorf("%s %s", errUnableToAttachVNIC, err)
	}

	vnic, err := n.manager.api.WaitVNICAttached(ctx, attachmentID)
	if err != nil {
		log.WithFields(logrus.Fields{
			"vnicAttachmentID": attachmentID,
			"instanceID":       instanceID,
		}).Error("Wait for VNIC attach failed")

		// TODO: detach
		return 0, errUnableToAttachVNIC, fmt.Errorf("%s %s", errUnableToAttachVNIC, err)
	}

	if vnic == nil {
		log.WithFields(logrus.Fields{
			"vnicAttachmentID": attachmentID,
			"instanceID":       instanceID,
		}).Error("Wait for VNIC attach failed, returned nil vnic")
		return 0, errUnableToAttachVNIC, fmt.Errorf("returned nil vnic")
	}

	n.vnics[vnic.ID] = *vnic
	scopedLog.WithField(fieldVNICID, vnic.ID).Info("Attached VNIC to instance")

	// Add the information of the created VNIC to the instances manager
	n.manager.UpdateVNIC(instanceID, vnic)
	return toAllocate, "", nil
}

// ResyncInterfacesAndIPs is called to retrieve and VNICs and IPs as known to
// the OCI API and return them
// func (n *Node) ResyncInterfacesAndIPs(ctx context.Context, scopedLog *logrus.Entry) (available ipamTypes.AllocationMap, remainAvailableVNICsCount int, err error) {
// 	l, limitsAvailable := n.getLimits()
// 	if !limitsAvailable {
// 		return nil, -1, fmt.Errorf(errUnableToDetermineLimits)
// 	}

// 	instanceID := n.node.InstanceID()
// 	available = ipamTypes.AllocationMap{}

// 	n.mutex.Lock()
// 	defer n.mutex.Unlock()
// 	n.vnics = map[string]vnicTypes.VNIC{}

// 	n.manager.ForeachInstance(instanceID,
// 		func(instanceID, interfaceID string, rev ipamTypes.InterfaceRevision) error {
// 			e, ok := rev.Resource.(*vnicTypes.VNIC)
// 			if !ok {
// 				log.Info("rev.Resource.(*vnicTypes.VNIC) failed")
// 				return nil
// 			}

// 			n.vnics[e.ID] = *e
// 			if e.IsPrimary {
// 				log.Debug("Skip primary VNIC for OCI ResyncInterfacesAndIPs")
// 				return nil
// 			}

// 			availableOnENI := math.IntMax(l.IPv4-len(e.Addresses), 0)
// 			if availableOnENI > 0 {
// 				remainAvailableVNICsCount++
// 			}

// 			for _, ip := range e.Addresses {
// 				available[ip] = ipamTypes.AllocationIP{Resource: e.ID}
// 			}

// 			return nil
// 		})

// 	vnics := len(n.vnics)

// 	// An OCI instance has at least one VNIC attached, no VNIC found implies instance not found.
// 	if vnics == 0 {
// 		scopedLog.Warning("OCI instance not found! Please delete corresponding ciliumnode if instance has already been deleted.")
// 		return nil, -1, fmt.Errorf("unable to retrieve VNICs")
// 	}
// 	remainAvailableVNICsCount += l.Adapters - len(n.vnics)

//		return available, remainAvailableVNICsCount, nil
//	}
func (n *Node) ResyncInterfacesAndIPs(ctx context.Context, log *logrus.Entry) (types.AllocationMap, stats.InterfaceStats, error) {
	l, limitsAvailable := n.getLimits()
	if !limitsAvailable {
		return nil, stats.InterfaceStats{}, fmt.Errorf(errUnableToDetermineLimits)
	}

	available := types.AllocationMap{}
	interfaceStats := stats.InterfaceStats{}

	// Try to use VNIC data from CiliumNode status first (populated by OCI sync)
	// This avoids repeated OCI API calls and permission issues
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.k8sObj != nil && n.k8sObj.Status.OCI.VNICs != nil && len(n.k8sObj.Status.OCI.VNICs) > 0 {
		// Use cached VNIC data from status
		log.Debug("Using VNIC data from CiliumNode status")
		n.vnics = map[string]vnicTypes.VNIC{}
		for vnicID, vnic := range n.k8sObj.Status.OCI.VNICs {
			n.vnics[vnicID] = vnic

			// Build available map from VNIC addresses
			for _, addr := range vnic.Addresses {
				// Only add to available pool if it's NOT the primary VNIC's primary IP
				if !vnic.IsPrimary || addr != vnic.PrimaryIP {
					available[addr] = types.AllocationIP{Resource: vnicID}
				}
			}
		}

		// Update interface stats
		interfaceStats.NodeCapacity = l.IPv4 * l.Adapters
		interfaceStats.RemainingAvailableInterfaceCount = l.Adapters - len(n.vnics)

		return available, interfaceStats, nil
	}

	// Fallback to OCI API calls if status data not available
	log.Debug("Falling back to OCI API calls for VNIC data")
	n.vnics = map[string]vnicTypes.VNIC{} // Reset vnics map

	instance, err := n.manager.api.GetInstance(ctx, n.instanceID)
	if err != nil {
		return nil, interfaceStats, err
	}

	vnicAttachments, err := n.manager.api.GetVnicAttachments(ctx, instance.CompartmentId, &instance.Id)
	if err != nil {
		return nil, interfaceStats, err
	}

	nonPrimaryVNICs := 0

	for _, vnicAttachment := range vnicAttachments {
		v, err := n.manager.api.GetVnic(ctx, vnicAttachment.VnicId)
		if err != nil {
			return nil, interfaceStats, err
		}

		// Get VCN ID from subnet mapping
		vcnID := ""
		subnetID := *v.SubnetId
		if subnet := n.manager.GetSubnet(subnetID); subnet != nil {
			vcnID = subnet.VirtualNetworkID
		}

		// Initialize VNIC entry
		vnic := vnicTypes.VNIC{
			ID:        *v.Id,
			IsPrimary: *v.IsPrimary,
			Subnet: vnicTypes.OciSubnet{
				ID: subnetID,
			},
			VCN: vnicTypes.OciVCN{
				ID: vcnID,
			},
			Addresses: []string{},
		}

		if !vnic.IsPrimary {
			nonPrimaryVNICs++
		}

		// Add primary IP if available
		// Note: We add the primary IP to the vnic.Addresses list for tracking,
		// but we should NOT make it available for Pod allocation (it's used by the host).
		// The IPAM allocator will filter this out based on the node's instance IP.
		if v.PrivateIp != nil {
			// Only add to available pool if it's a secondary IP, not the primary VNIC's primary IP
			if !(*v.IsPrimary) {
				available[*v.PrivateIp] = types.AllocationIP{Resource: *v.Id}
			}
			vnic.Addresses = append(vnic.Addresses, *v.PrivateIp)
		}

		// Add secondary IPs
		if v.Id != nil {
			privateIPs, err := n.manager.api.ListPrivateIPs(ctx, *v.Id)
			if err != nil {
				return nil, interfaceStats, err
			}

			for _, privateIP := range privateIPs {
				if privateIP.IpAddress != nil {
					// Only add to available pool if it's NOT the primary VNIC's primary IP
					// This matches the logic in the CiliumNode status cache path
					if !(*v.IsPrimary && privateIP.IsPrimary != nil && *privateIP.IsPrimary) {
						available[*privateIP.IpAddress] = types.AllocationIP{Resource: *v.Id}
					}
					vnic.Addresses = append(vnic.Addresses, *privateIP.IpAddress)
				}
			}
		}

		// Store VNIC in the map
		n.vnics[vnic.ID] = vnic
	}

	// Update interface stats
	interfaceStats.NodeCapacity = l.IPv4 * l.Adapters
	interfaceStats.RemainingAvailableInterfaceCount = l.Adapters - len(n.vnics)

	return available, interfaceStats, nil
}

// func (n *Node) ResyncInterfacesAndIPs(ctx context.Context, log *logrus.Entry) (types.AllocationMap, stats.InterfaceStats, error) {
// 	available := types.AllocationMap{}
// 	stats := stats.InterfaceStats{}

// 	instance, err := n.manager.api.GetInstance(ctx, n.instanceID)
// 	if err != nil {
// 		return nil, stats, err
// 	}

// 	// 锁定以更新 vnics 映射
// 	n.mutex.Lock()
// 	defer n.mutex.Unlock()
// 	n.vnics = map[string]vnicTypes.VNIC{} // 重置 vnics 映射

// 	vnicAttachments, err := n.manager.api.GetVnicAttachments(ctx, instance.CompartmentId, &instance.Id)
// 	if err != nil {
// 		return nil, stats, err
// 	}

// 	for _, vnicAttachment := range vnicAttachments {
// 		v, err := n.manager.api.GetVnic(ctx, vnicAttachment.VnicId)
// 		if err != nil {
// 			return nil, stats, err
// 		}

// 		// 更新 vnics 映射
// 		n.vnics[*v.Id] = vnicTypes.VNIC{
// 			ID:        *v.Id,
// 			IsPrimary: *v.IsPrimary,
// 			Subnet: vnicTypes.Subnet{
// 				ID: *v.SubnetId,
// 			},
// 			VCN: vnicTypes.VCN{
// 				ID: instance.CompartmentId, // 或从其他地方获取 VCN ID
// 			},
// 			Addresses: []string{},
// 		}

// 		if v.PrivateIp != nil {
// 			available[*v.PrivateIp] = types.AllocationIP{Resource: *v.Id}
// 			n.vnics[*v.Id].Addresses = append(n.vnics[*v.Id].Addresses, *v.PrivateIp)
// 		}

// 		privateIPs, err := n.manager.api.ListPrivateIPs(ctx, v.Id)
// 		if err != nil {
// 			return nil, stats, err
// 		}

// 		for _, privateIP := range privateIPs {
// 			available[*privateIP.IpAddress] = types.AllocationIP{Resource: *v.Id}
// 			n.vnics[*v.Id].Addresses = append(n.vnics[*v.Id].Addresses, *privateIP.IpAddress)
// 		}
// 	}

// 	stats.AvailableIPs = len(available)
// 	return available, stats, nil
// }

// PrepareIPAllocation returns the number of VNIC IPs and interfaces that can be
// allocated/created.
func (n *Node) PrepareIPAllocation(scopedLog *logrus.Entry) (*ipam.AllocationAction, error) {
	l, limitsAvailable := n.getLimits()
	if !limitsAvailable {
		return nil, fmt.Errorf("Unable to determine limits")
	}

	a := &ipam.AllocationAction{}

	n.mutex.RLock()
	defer n.mutex.RUnlock()

	// Sort VNIC IDs for deterministic iteration order
	vnicIDs := make([]string, 0, len(n.vnics))
	for k := range n.vnics {
		vnicIDs = append(vnicIDs, k)
	}
	sort.Strings(vnicIDs)

	// First pass: collect all VNICs and find the best one with capacity
	bestVNICKey := ""
	bestAvailable := 0
	totalUsedIPs := 0

	for _, key := range vnicIDs {
		e := n.vnics[key]
		scopedLog.WithFields(logrus.Fields{
			fieldVNICID: e.ID,
			"ipv4Limit": l.IPv4,
			"allocated": len(e.Addresses),
			"isPrimary": e.IsPrimary,
		}).Debug("PrepareIPAllocation: considering VNIC for allocation")

		// Count used IPs on this VNIC from k8s status
		usedIPsOnVNIC := 0
		if n.k8sObj != nil && n.k8sObj.Status.IPAM.Used != nil {
			for _, allocation := range n.k8sObj.Status.IPAM.Used {
				if allocation.Resource == e.ID {
					usedIPsOnVNIC++
				}
			}
		}
		totalUsedIPs += usedIPsOnVNIC

		// Note: Unlike AWS ENI which skips primary ENI, OCI allows allocating
		// secondary private IPs to the primary VNIC. This is the recommended
		// approach for OCI to avoid unnecessary VNIC attachments.
		// We do NOT skip primary VNIC here.

		availableOnVNIC := math.IntMax(l.IPv4-len(e.Addresses), 0)
		if availableOnVNIC <= 0 {
			scopedLog.WithFields(logrus.Fields{
				fieldVNICID: e.ID,
				"allocated": len(e.Addresses),
				"limit":     l.IPv4,
			}).Debug("VNIC is at capacity, skipping")
			continue
		}

		a.InterfaceCandidates++
		scopedLog.WithFields(logrus.Fields{
			"availableOnVNIC": availableOnVNIC,
			"usedIPsOnVNIC":   usedIPsOnVNIC,
		}).Debug("VNIC has IPs available")

		if subnet := n.manager.GetSubnet(e.Subnet.ID); subnet != nil {
			if subnet.AvailableAddresses > 0 {
				available := math.IntMin(subnet.AvailableAddresses, availableOnVNIC)
				if available > bestAvailable {
					bestVNICKey = key
					bestAvailable = available
					a.PoolID = ipamTypes.PoolID(subnet.ID)
					scopedLog.WithFields(logrus.Fields{
						"subnetID":           e.Subnet.ID,
						"availableAddresses": subnet.AvailableAddresses,
						"bestAvailable":      bestAvailable,
					}).Debug("Found better VNIC candidate")
				}
			}
		}
	}

	// Set the best VNIC if found
	if bestVNICKey != "" && bestAvailable > 0 {
		a.InterfaceID = bestVNICKey
		a.AvailableForAllocation = bestAvailable
		scopedLog.WithFields(logrus.Fields{
			"selectedVNIC":           bestVNICKey,
			"availableForAllocation": a.AvailableForAllocation,
		}).Info("Selected VNIC for IP allocation")
	}

	a.EmptyInterfaceSlots = l.Adapters - len(n.vnics)
	log.WithFields(logrus.Fields{
		"EmptyInterfaceSlots":    a.EmptyInterfaceSlots,
		"InterfaceID":            a.InterfaceID,
		"AvailableForAllocation": a.AvailableForAllocation,
		"totalUsedIPs":           totalUsedIPs,
	}).Info("PrepareIPAllocation completed")

	return a, nil
}

// AllocateIPs performs the VNIC allocation operation
func (n *Node) AllocateIPs(ctx context.Context, a *ipam.AllocationAction) error {
	log.WithFields(logrus.Fields{
		"vnicID":     a.InterfaceID,
		"toAllocate": a.AvailableForAllocation,
	}).Info("AllocateIPs for VNIC")

	// NOTE: OCI doesn't support assign/create multiple IP addresses in single request,
	// so we allocate with a loop
	for i := 0; i < a.AvailableForAllocation; i++ {
		_, err := n.manager.api.AssignPrivateIPAddresses(ctx, a.InterfaceID, 1)
		if err != nil {
			return err
		}
	}

	return nil
}

// PrepareIPRelease prepares the release of VNIC IPs.
func (n *Node) PrepareIPRelease(excessIPs int, scopedLog *logrus.Entry) *ipam.ReleaseAction {
	r := &ipam.ReleaseAction{}

	n.mutex.Lock()
	defer n.mutex.Unlock()

	// Sort VNIC IDs for deterministic selection when multiple VNICs
	// have IPs available for release (similar to AWS ENI implementation)
	vnicIDs := make([]string, 0, len(n.vnics))
	for k := range n.vnics {
		vnicIDs = append(vnicIDs, k)
	}
	sort.Strings(vnicIDs)

	// Iterate over VNICs on this node, select the VNIC with the most
	// addresses available for release
	for _, key := range vnicIDs {
		e := n.vnics[key]
		scopedLog.WithFields(logrus.Fields{
			fieldVNICID:    e.ID,
			"numAddresses": len(e.Addresses),
			"isPrimary":    e.IsPrimary,
		}).Debug("Considering VNIC for IP release")

		// Note: For OCI, we can release secondary private IPs from the primary VNIC.
		// This is different from AWS where primary ENI is typically not managed.
		// We do NOT skip primary VNIC here.

		// Count free IP addresses on this VNIC
		ipsOnVNIC := n.k8sObj.Status.OCI.VNICs[e.ID].Addresses
		freeIpsOnVNIC := []string{}
		for _, ip := range ipsOnVNIC {
			_, ipUsed := n.k8sObj.Status.IPAM.Used[ip]
			// Exclude primary VNIC's primary IP from release candidates
			// to prevent releasing the node's main IP address
			if !ipUsed && !(e.IsPrimary && ip == e.PrimaryIP) {
				freeIpsOnVNIC = append(freeIpsOnVNIC, ip)
			}
		}
		freeOnVNICCount := len(freeIpsOnVNIC)

		if freeOnVNICCount <= 0 {
			continue
		}

		scopedLog.WithFields(logrus.Fields{
			fieldVNICID:       e.ID,
			"excessIPs":       excessIPs,
			"freeOnVNICCount": freeOnVNICCount,
		}).Debug("VNIC has unused IPs that can be released")
		maxReleaseOnVNIC := math.IntMin(freeOnVNICCount, excessIPs)

		r.InterfaceID = key
		// Use subnet ID as pool ID, which is consistent with IPAM pool management
		r.PoolID = ipamTypes.PoolID(e.Subnet.ID)
		r.IPsToRelease = freeIpsOnVNIC[:maxReleaseOnVNIC]
	}

	return r
}

// ReleaseIPs performs the VNIC IP release operation
func (n *Node) ReleaseIPs(ctx context.Context, r *ipam.ReleaseAction) error {
	return n.manager.api.UnassignPrivateIPAddresses(ctx, r.InterfaceID, r.IPsToRelease)
}

// GetMaximumAllocatableIPv4 returns the maximum amount of IPv4 addresses
// that can be allocated to the instance
func (n *Node) GetMaximumAllocatableIPv4() int {
	return 32

	// n.mutex.RLock()
	// defer n.mutex.RUnlock()

	/*
		// Retrieve l for the instance type
		l, limitsAvailable := n.getLimitsLocked()
		if !limitsAvailable {
			return 0
		}

		// Return the maximum amount of IP addresses allocatable on the instance
		// reserve Primary eni
		return (l.Adapters - 1) * l.IPv4
	*/
}

// GetMinimumAllocatableIPv4 returns the minimum amount of IPv4 addresses that
// must be allocated to the instance.
func (n *Node) GetMinimumAllocatableIPv4() int {
	return defaults.IPAMPreAllocation
}

func (n *Node) loggerLocked() *logrus.Entry {
	if n == nil || n.instanceID == "" {
		return log
	}

	return log.WithField("instanceID", n.instanceID)
}

// getLimits returns the interface and IP limits of this node
func (n *Node) getLimits() (ipamTypes.Limits, bool) {
	n.mutex.RLock()
	l, b := n.getLimitsLocked()
	n.mutex.RUnlock()
	return l, b
}

// getLimitsLocked is the same function as getLimits, but assumes the n.mutex
// is read locked.
func (n *Node) getLimitsLocked() (ipamTypes.Limits, bool) {
	l, ok := limits.Get(n.k8sObj.Spec.OCI.Shape)
	if !ok {
		return l, ok
	}

	// 打印形状级上限
	log.WithFields(logrus.Fields{
		"shape":             n.k8sObj.Spec.OCI.Shape,
		"adaptersFromShape": l.Adapters,
		"ipv4PerAdapter":    l.IPv4,
		"instanceID":        n.node.InstanceID(),
	}).Info("getLimitsLocked: shape-level limits")

	// 用实例级 shape-config 的上限覆盖静态形状上限（Flex 形状尤其关键）
	if n.manager != nil && n.manager.api != nil {
		if max, err := n.manager.api.GetInstanceMaxVnicAttachments(context.TODO(), n.node.InstanceID()); err == nil && max > 0 {
			if max != l.Adapters {
				log.WithFields(logrus.Fields{
					"shape":                n.k8sObj.Spec.OCI.Shape,
					"adaptersFromShape":    l.Adapters,
					"adaptersFromInstance": max,
					"instanceID":           n.node.InstanceID(),
				}).Info("getLimitsLocked: overriding adapters with instance-level maxVnicAttachments")
			}
			l.Adapters = max
		} else if err != nil {
			log.WithError(err).Warn("getLimitsLocked: failed to query instance max VNIC attachments")
		}
	}

	return l, true
}

func (n *Node) getSecurityGroupIDs(ctx context.Context, eniSpec vnicTypes.OciSpec) ([]string, error) {
	// VNIC must have at least one security group
	// 1. use security group defined by user
	// 2. use security group used by primary VNIC (eth0)

	if len(eniSpec.SecurityGroups) > 0 {
		return eniSpec.SecurityGroups, nil
	}

	if len(eniSpec.SecurityGroupTags) > 0 {
		securityGroups := n.manager.FindSecurityGroupByTags(eniSpec.VCNID, eniSpec.SecurityGroupTags)
		if len(securityGroups) == 0 {
			n.loggerLocked().WithFields(logrus.Fields{
				"vcnID": eniSpec.VCNID,
				"tags":  eniSpec.SecurityGroupTags,
			}).Warn("No security groups match required VCN ID and tags, using primary VNIC's security groups")
		} else {
			groups := make([]string, 0, len(securityGroups))
			for _, secGroup := range securityGroups {
				groups = append(groups, secGroup.ID)
			}
			return groups, nil
		}
	}

	var securityGroups []string

	n.manager.ForeachInstance(n.node.InstanceID(),
		func(instanceID, interfaceID string, rev ipamTypes.InterfaceRevision) error {
			// e, ok := rev.Resource.(*vnicTypes.VNIC)
			// if ok && e.Type == vnicTypes.VNICTypePrimary {
			// 	securityGroups = append(securityGroups, e.SecurityGroupIDs...)
			// }
			return nil
		})

	if len(securityGroups) <= 0 {
		return nil, fmt.Errorf("failed to get security group ids")
	}

	return securityGroups, nil
}

// allocVNICIndex will alloc an monotonically increased index for each VNIC on this instance.
// The index generated the first time this VNIC is created, and stored in VNIC.Tags.
func (n *Node) allocVNICIndex() (int, error) {
	// alloc index for each created VNIC
	used := make([]bool, maxVNICPerNode)
	// TODO: implement this

	// for _, v := range n.vnics {
	// 	index := utils.GetVNICIndexFromTags(v.Tags)
	// 	if index > maxVNICPerNode || index < 0 {
	// 		return 0, fmt.Errorf("VNIC index(%d) is out of range", index)
	// 	}
	// 	used[index] = true
	// }
	// ECS has at least 1 VNIC, 0 is reserved for eth0
	i := 1
	for ; i < maxVNICPerNode; i++ {
		if !used[i] {
			break
		}
	}
	return i, nil
}

func (n *Node) IsPrefixDelegated() bool {
	return false
}

func (n *Node) GetUsedIPWithPrefixes() int {
	if n.k8sObj == nil {
		return 0
	}
	return len(n.k8sObj.Status.IPAM.Used)
}
