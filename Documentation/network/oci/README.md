# Cilium OCI IPAM

Oracle Cloud Infrastructure (OCI) IPAM integration for Cilium.

## Overview

Cilium's OCI IPAM mode enables native integration with Oracle Cloud Infrastructure networking. Instead of using Cilium's built-in IPAM, pods receive IP addresses directly from OCI VCN (Virtual Cloud Network) subnets through VNIC (Virtual Network Interface Card) allocation.

## Key Features

- ✅ **Native OCI Networking**: Pods get IPs from OCI VCN subnets
- ✅ **Dynamic VNIC Management**: Automatic VNIC creation and attachment
- ✅ **Instance Principal Auth**: Secure authentication using OCI instance principals
- ✅ **Flexible Scaling**: Automatic IP allocation based on pod demand
- ✅ **Multi-Subnet Support**: Distribute pods across multiple subnets
- ✅ **Shape-Aware Limits**: Automatically detects instance VNIC limits

## How It Works

```
┌─────────────────────────────────────────────────────────┐
│                    OCI Worker Node                       │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   Pod A      │  │   Pod B      │  │   Pod C      │  │
│  │ 10.0.1.10    │  │ 10.0.1.11    │  │ 10.0.2.10    │  │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │
│         │                 │                 │           │
│         └────────┬────────┘                 │           │
│                  │                          │           │
│         ┌────────▼────────┐       ┌─────────▼────────┐  │
│         │   VNIC 1        │       │   VNIC 2         │  │
│         │   Primary       │       │   Secondary      │  │
│         │   10.0.1.5      │       │   10.0.2.5       │  │
│         │   (eth0)        │       │   (eth1)         │  │
│         └────────┬────────┘       └─────────┬────────┘  │
│                  │                          │           │
└──────────────────┼──────────────────────────┼───────────┘
                   │                          │
                   └──────────┬───────────────┘
                              │
                   ┌──────────▼──────────┐
                   │    OCI VCN          │
                   │    10.0.0.0/16      │
                   │                     │
                   │  ┌──────────────┐   │
                   │  │  Subnet 1    │   │
                   │  │  10.0.1.0/24 │   │
                   │  └──────────────┘   │
                   │  ┌──────────────┐   │
                   │  │  Subnet 2    │   │
                   │  │  10.0.2.0/24 │   │
                   │  └──────────────┘   │
                   └─────────────────────┘
```

### IPAM Flow

1. **Pod Creation Request** → Cilium detects a new pod needs an IP
2. **VNIC Selection** → Cilium checks existing VNICs for available IPs
3. **IP Allocation**:
   - If available: Assign secondary IP to existing VNIC
   - If full: Create new VNIC in suitable subnet
4. **Pod Network Setup** → Configure pod network interface with allocated IP
5. **Route Updates** → Update routing tables for pod connectivity

## Documentation

- **[Quick Start Guide](quickstart.md)** - Get started with OCI IPAM in 5 steps
- **[Troubleshooting Guide](troubleshooting.md)** - Common issues and solutions
- **[Configuration Reference](configuration.md)** - Detailed configuration options

## When to Use OCI IPAM

### ✅ Use OCI IPAM when:

- You need pods to communicate directly with OCI resources (databases, VMs, etc.)
- You want to leverage OCI Network Security Groups for pod-level security
- Your organization requires all IPs to come from managed VCN subnets
- You need consistent IP addressing across Kubernetes and non-Kubernetes workloads
- You want to use OCI native load balancers with pod IPs

### ❌ Consider alternatives when:

- You have a small VCN CIDR and need to conserve IPs
- You want pod IPs to be completely isolated from infrastructure
- You need more than 32 IPs per VNIC per node
- Your cluster is not running on OCI

## Requirements

### Infrastructure
- OCI Kubernetes cluster (OKE) or self-managed Kubernetes on OCI compute instances
- Kubernetes version 1.23+
- OCI VCN with sufficient IP space
- Multiple subnets (recommended) for redundancy

### Permissions
- Instance principals or OCI config file with appropriate IAM policies
- Permissions to manage VNICs, private IPs, and query VCN resources

### Cilium
- Cilium version 1.15.2+
- Built with `ipam_provider_oci` tag

## Quick Example

Minimal Helm configuration:

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."

OCIUseInstancePrincipal: true

operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...
```

Install:

```bash
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values values.yaml
```

## Performance Considerations

### VNIC Limits

Each OCI instance shape has limits on:
- **Maximum VNICs**: Varies by shape (typically 2-24)
- **IPs per VNIC**: 32 secondary IPs (plus 1 primary)

Example capacity:
```
VM.Standard.E4.Flex (4 OCPU): 2 VNICs × 32 IPs = 64 pods per node
BM.Standard.E4.128: 24 VNICs × 32 IPs = 768 pods per node
```

### Scaling Behavior

- **First IP allocation**: ~2-3 seconds (create VNIC)
- **Subsequent IPs on same VNIC**: ~500ms
- **New VNIC creation**: ~3-5 seconds
- **VNIC attachment**: ~5-10 seconds

### Optimization Tips

1. **Pre-allocate VNICs**: Set higher pre-allocation thresholds
2. **Use multiple subnets**: Distribute load across availability domains
3. **Monitor VNIC usage**: Set up alerts for VNIC exhaustion
4. **Right-size shapes**: Choose shapes with adequate VNIC limits

## Comparison with Other IPAM Modes

| Feature | OCI IPAM | Cluster Pool | Kubernetes Host Scope |
|---------|----------|--------------|----------------------|
| IP Source | OCI VCN subnets | Cilium-managed pool | Node PodCIDR |
| Pod-to-OCI latency | Lowest | Medium | Medium |
| IP conservation | Medium | High | Medium |
| OCI integration | Native | None | None |
| Complexity | Medium | Low | Low |
| Scalability | Shape-limited | Unlimited | Node-limited |

## Architecture Details

### Components

- **Cilium Operator**: Manages IPAM for the cluster
  - Discovers OCI instance shapes and limits
  - Allocates IPs from VCN subnets
  - Creates and attaches VNICs as needed

- **Cilium Agent**: Runs on each node
  - Reports node IPAM status via CiliumNode CRD
  - Configures pod network interfaces
  - Maintains local IP allocation state

- **OCI API Integration**: 
  - Virtual Network API for VNIC management
  - Compute API for instance queries
  - Resource Search API for VCN discovery

### CRD Schema

CiliumNode spec and status for OCI:

```yaml
apiVersion: cilium.io/v2
kind: CiliumNode
metadata:
  name: node-1
spec:
  oci:
    vcn-id: "ocid1.vcn.oc1.phx.xxx"
    availability-domain: "AD-1"
    subnet-tags:
      environment: production
status:
  oci:
    vnics:
      "ocid1.vnic.oc1.phx.xxx":
        id: "ocid1.vnic.oc1.phx.xxx"
        mac: "02:00:17:xx:xx:xx"
        primary-ip: "10.0.1.5"
        is-primary: true
        addresses:
          - "10.0.1.5"
          - "10.0.1.10"
          - "10.0.1.11"
        subnet:
          id: "ocid1.subnet.oc1.phx.xxx"
          cidr: "10.0.1.0/24"
        vcn:
          id: "ocid1.vcn.oc1.phx.xxx"
          cidr-blocks:
            - "10.0.0.0/16"
```

## Security Considerations

### IAM Best Practices

1. **Use Instance Principals**: Avoid storing credentials
2. **Least Privilege**: Grant only required permissions
3. **Compartment Isolation**: Use separate compartments for different environments
4. **Audit Logging**: Enable OCI audit logs for VNIC operations

### Network Security

1. **Security Lists**: Apply security lists to subnets
2. **Network Security Groups**: Use NSGs for fine-grained pod security
3. **Private Subnets**: Use private subnets for pod IPs
4. **Route Tables**: Configure proper routing for pod traffic

## Getting Help

- **Documentation**: See [quickstart.md](quickstart.md) and [troubleshooting.md](troubleshooting.md)
- **Logs**: `kubectl -n kube-system logs deployment/cilium-operator`
- **Status**: `kubectl get ciliumnodes -o yaml`
- **GitHub Issues**: Report bugs and request features

## Contributing

Contributions are welcome! See the main [Cilium contributing guide](../../../CONTRIBUTING.md).

## License

Apache License 2.0 - See [LICENSE](../../../LICENSE)
