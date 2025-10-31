# Cilium OCI IPAM

Oracle Cloud Infrastructure (OCI) IPAM is a feature that enables Cilium to automatically allocate IP addresses for Kubernetes Pods directly from OCI VCN subnets using the OCI API.

**Version**: Cilium v1.15.2  
**Status**: Production Ready  
**Last Updated**: October 27, 2025

---

## Overview

### What is OCI IPAM?

OCI IPAM (IP Address Management) allows Cilium to:

- **Allocate Pod IPs from OCI VCN subnets** - Pods receive native OCI IP addresses
- **Automatically manage VNICs** - Creates and manages multiple VNICs per node
- **Support Instance Principal authentication** - Secure, keyless authentication
- **Auto-create VNICs via Subnet Tags** - Fully automated VNIC management

### Key Benefits

| Benefit | Description |
|---------|-------------|
| **Native OCI Integration** | Pods use real OCI VCN IP addresses, enabling seamless integration with OCI services |
| **Automatic Scaling** | VNICs are created automatically when IP addresses are exhausted |
| **Flexible Subnet Management** | Use Subnet Tags to control which subnets Cilium can use |
| **High Density** | Support for 100+ Pods per node (limited by instance shape) |
| **Network Policy Compatible** | Full Cilium Network Policy support with OCI IP addresses |

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                 Cilium Operator                          │
│  ┌────────────────────────────────────────────────┐     │
│  │  OCI IPAM Allocator                            │     │
│  │  - Query VCN subnets                           │     │
│  │  - Create/manage VNICs                         │     │
│  │  - Allocate secondary IPs                      │     │
│  └────────────────┬───────────────────────────────┘     │
│                   │ OCI SDK                              │
└───────────────────┼──────────────────────────────────────┘
                    │
         ┌──────────▼──────────┐
         │    OCI VCN API      │
         │  - Virtual Network  │
         │  - Compute API      │
         └─────────────────────┘
                    │
         ┌──────────▼──────────┐
         │   OCI Worker Node   │
         │  ┌──────────────┐   │
         │  │ VNIC 1       │   │
         │  │ eth0 (主网卡)│   │
         │  │ + 32 IPs     │   │
         │  └──────────────┘   │
         │  ┌──────────────┐   │
         │  │ VNIC 2       │   │
         │  │ eth1 (辅助)  │   │
         │  │ + 32 IPs     │   │
         │  └──────────────┘   │
         └─────────────────────┘
```

### Components

1. **Cilium Operator**: Manages OCI IPAM globally
   - Queries available subnets
   - Creates VNICs when needed
   - Allocates IP addresses to nodes

2. **Cilium Agent**: Runs on each node
   - Assigns IPs to Pods
   - Manages local network configuration

3. **OCI Metadata Service**: Provides instance information
   - Instance OCID
   - Compartment ID
   - Available VNICs

---

## Quick Start

### Prerequisites

- Kubernetes 1.21+
- OCI VCN with subnets
- Instance Principal authentication configured
- Sufficient OCI IAM permissions

### Basic Installation

```bash
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --set ipam.mode=oci \
  --set oci.enabled=true \
  --set oci.useInstancePrincipal=true \
  --set oci.vcnID="ocid1.vcn.oc1..."
```

### With Subnet Tags (Recommended)

```bash
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --set ipam.mode=oci \
  --set oci.enabled=true \
  --set oci.useInstancePrincipal=true \
  --set oci.vcnID="ocid1.vcn.oc1..." \
  --set oci.subnetTags.cilium-pod-network="yes" \
  --set-string 'operator.extraArgs={--subnet-tags-filter=cilium-pod-network=yes}'
```

**See**: [quickstart.md](quickstart.md) for detailed step-by-step instructions.

---

## Key Features

### 1. Automatic VNIC Management

Cilium automatically creates and manages VNICs:

- **Threshold-based creation**: Create VNIC when IP usage reaches 80%
- **Subnet selection**: Choose subnets based on availability or tags
- **Graceful handling**: Respects instance shape VNIC limits

### 2. Subnet Tags

Control which subnets Cilium uses with Freeform Tags:

```yaml
# Helm configuration
oci:
  subnetTags:
    cilium-pod-network: "yes"

operator:
  extraArgs:
    - --subnet-tags-filter=cilium-pod-network=yes
```

**Important**: Both configurations are required due to Cilium's architecture (Agent vs Operator).

### 3. Instance Principal Authentication

Secure, keyless authentication using OCI Instance Principals:

- No API keys to manage
- Automatic credential rotation
- Recommended for production environments

### 4. IP Allocation Strategies

- **Pre-allocation**: Pre-create VNICs before IP exhaustion
- **Surge allocation**: Allocate multiple IPs at once for scaling
- **Efficient recycling**: Reuse IPs from deleted Pods

---

## Configuration

### Basic Configuration

```yaml
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"  # Match your VCN CIDR

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn.oc1.region..."
  
  # VNIC management
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32
```

**See**: [configuration.md](configuration.md) for all available options.

---

## Common Use Cases

### Use Case 1: Standard Deployment

**Scenario**: Deploy Cilium with basic OCI IPAM  
**Configuration**: Use existing VNICs, manual VNIC creation if needed

### Use Case 2: Auto-Scaling Workloads

**Scenario**: Workloads scale up/down frequently  
**Configuration**: Enable Subnet Tags for automatic VNIC creation

```yaml
oci:
  subnetTags:
    cilium-pod-network: "yes"
  vnicPreAllocationThreshold: 0.8
```

### Use Case 3: High-Density Pods

**Scenario**: Need 100+ Pods per node  
**Configuration**: Multiple VNICs with large subnets

```yaml
oci:
  maxIPsPerVNIC: 32
  # Use /24 or larger subnets (250+ IPs)
```

---

## Troubleshooting

### Common Issues

#### Issue 1: Pods stuck in ContainerCreating

**Symptoms**: Pods cannot get IP addresses

**Diagnosis**:
```bash
kubectl get ciliumnode
kubectl logs -n kube-system deployment/cilium-operator
```

**Possible Causes**:
- IP exhaustion in subnets
- IAM permission issues
- VNIC limit reached

**See**: [troubleshooting.md](troubleshooting.md) for detailed solutions.

#### Issue 2: Subnet Tags not working

**Symptoms**: `--subnet-tags-filter=''` in Operator logs

**Root Cause**: Missing `operator.extraArgs` configuration

**Solution**: Configure both places:
```yaml
oci:
  subnetTags:
    key: "value"

operator:
  extraArgs:
    - --subnet-tags-filter=key=value
```

---

## Best Practices

### 1. Subnet Planning

✅ **Use /24 or larger subnets** - Provides 250+ IPs  
✅ **Create subnets in multiple ADs** - High availability  
❌ **Avoid /28 subnets** - Only 13 usable IPs, causes issues

### 2. IAM Configuration

✅ **Use Instance Principal** - More secure than API keys  
✅ **Scope policies to specific compartments** - Principle of least privilege  
✅ **Test permissions** - Verify with `oci iam region list --auth instance_principal`

### 3. Monitoring

Monitor these key metrics:

- `cilium_oci_subnet_ips_used / cilium_oci_subnet_ips_total` - Subnet IP usage
- `cilium_oci_vnic_creation_errors_total` - VNIC creation failures
- `cilium_ipam_allocation_duration_seconds` - IP allocation latency

### 4. Production Configuration

```yaml
# Recommended production configuration
oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn..."
  subnetTags:
    cilium-pod-network: "yes"
    environment: "production"
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32

operator:
  replicas: 2  # High availability
  extraArgs:
    - --subnet-tags-filter=cilium-pod-network=yes

prometheus:
  enabled: true

hubble:
  enabled: true
  relay:
    enabled: true
```

---

## Performance

### Expected Performance

| Metric | Value | Notes |
|--------|-------|-------|
| **IP Allocation Time** | < 2s | Per Pod |
| **VNIC Creation Time** | 15-20s | First time |
| **Pod Creation Rate** | 5 Pods/sec | Limited by OCI API |
| **Max Pods/Node** | 100+ | Depends on instance shape |

### Tuning Parameters

```yaml
oci:
  vnicPreAllocationThreshold: 0.8     # Pre-create at 80%
  ipAllocationTimeout: 60             # 60 second timeout
  maxIPsPerVNIC: 32                   # OCI limit
```

---

## Security

### IAM Permissions Required

```
Allow dynamic-group <group-name> to manage vnics in compartment <compartment>
Allow dynamic-group <group-name> to use subnets in compartment <compartment>
Allow dynamic-group <group-name> to use network-security-groups in compartment <compartment>
Allow dynamic-group <group-name> to use private-ips in compartment <compartment>
```

### Network Security

- Pod IPs are real OCI IPs - can apply Network Security Groups
- Cilium Network Policies work with OCI IPs
- VCN security lists apply to Pod traffic

---

## Limitations

| Limitation | Impact | Mitigation |
|------------|--------|------------|
| **Instance shape VNIC limit** | Max 2-24 VNICs depending on shape | Choose appropriate instance shape |
| **OCI API rate limits** | Slower Pod creation during scaling | Use pre-allocation |
| **Subnet size** | /28 only provides 13 IPs | Use /24 or larger |
| **Single region** | No cross-region support | Deploy per region |

---

## Documentation

### Core Documentation

- [quickstart.md](quickstart.md) - Quick start guide
- [configuration.md](configuration.md) - Complete configuration reference
- [troubleshooting.md](troubleshooting.md) - Troubleshooting guide


---

## Support

### Getting Help

- **GitHub Issues**: https://github.com/iafboy/oci_cilium/issues


### Reporting Issues

When reporting issues, include:

1. Cilium version and configuration
2. CiliumNode status: `kubectl get ciliumnode -o yaml`
3. Operator logs: `kubectl logs -n kube-system deployment/cilium-operator`
4. Agent logs: `kubectl logs -n kube-system daemonset/cilium`

---

## License

This documentation and the Cilium OCI IPAM feature are licensed under Apache 2.0.

---

**Version**: 2.0  
**Last Updated**: October 27, 2025  

