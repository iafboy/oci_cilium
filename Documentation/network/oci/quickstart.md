# OCI IPAM Quick Start Guide

This guide will help you set up Cilium with OCI IPAM on Oracle Cloud Infrastructure.

## Prerequisites

- OCI Kubernetes cluster (OKE) or self-managed Kubernetes on OCI compute instances
- Kubectl access to the cluster
- Helm 3.x installed
- Administrator access to OCI console for IAM policy configuration

## Architecture Overview

Cilium OCI IPAM mode allocates IP addresses from OCI VNICs (Virtual Network Interface Cards) attached to worker nodes. This provides native OCI networking integration with the following benefits:

- **Native OCI networking**: Pods get IPs from OCI VCN subnets
- **Direct communication**: Pods can communicate with other OCI resources without NAT
- **Flexible scaling**: Automatic VNIC and IP allocation based on pod demands
- **Security groups**: Leverage OCI Network Security Groups

## Step 1: Prepare OCI IAM Policies

### Option A: Using Instance Principals (Recommended)

1. **Create a Dynamic Group** in OCI Console:

   Navigate to: Identity & Security → Identity → Dynamic Groups

   ```hcl
   # Match all instances in a specific compartment
   matching_rule = "ALL {instance.compartment.id = 'ocid1.compartment.oc1..xxx'}"
   
   # OR match instances by tag
   matching_rule = "ALL {tag.namespace.key = 'cilium-operator'}"
   ```

2. **Create IAM Policies**:

   Navigate to: Identity & Security → Identity → Policies

   ```hcl
   # Required policies for Cilium Operator
   Allow dynamic-group cilium-operator-group to manage vnics in compartment <your-compartment>
   Allow dynamic-group cilium-operator-group to use subnets in compartment <your-compartment>
   Allow dynamic-group cilium-operator-group to use private-ips in compartment <your-compartment>
   Allow dynamic-group cilium-operator-group to read virtual-network-family in compartment <your-compartment>
   Allow dynamic-group cilium-operator-group to read compute-management-family in compartment <your-compartment>
   ```

### Option B: Using OCI Config File

If you cannot use Instance Principals, configure OCI SDK credentials:

1. Place your OCI config file on operator nodes at: `~/.oci/config`
2. Ensure the config file contains valid credentials
3. Set `OCIUseInstancePrincipal: false` in Helm values

## Step 2: Gather Required Information

You need the following OCI identifiers:

```bash
# Get your VCN ID
oci network vcn list --compartment-id <compartment-ocid> --query "data[0].id"
# Output: ocid1.vcn.oc1.phx.xxx

# Get your Compartment ID
oci iam compartment list --query "data[0].id"
# Output: ocid1.compartment.oc1..xxx

# List available subnets in your VCN
oci network subnet list --compartment-id <compartment-ocid> --vcn-id <vcn-ocid>
```

**Important**: Note your VCN OCID - you'll need it for the Helm configuration.

## Step 3: Install Cilium with OCI IPAM

### Prepare Helm Values

Create a file named `cilium-oci-values.yaml`:

```yaml
# Basic Configuration
ipam:
  mode: "oci"

# OCI-specific Configuration
oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"  # REQUIRED: Replace with your VCN OCID
  
# Use Instance Principal Authentication (default: true)
OCIUseInstancePrincipal: true

# Operator Configuration
operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx  # REQUIRED: Replace with your VCN OCID
  
  # Optional: Increase parallel workers for large clusters
  # extraArgs:
  #   - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
  #   - --parallel-alloc-workers=10

# Kubernetes Configuration
k8sServiceHost: <your-k8s-api-server-ip>
k8sServicePort: 6443

# Optional: Enable specific features
enableIPv4Masquerade: true
enableIPv6: false

# Optional: Configure tunnel mode (recommended for OCI)
tunnelProtocol: vxlan
routingMode: tunnel

# Optional: Enable Hubble for observability
hubble:
  enabled: true
  relay:
    enabled: true
  ui:
    enabled: true
```

### Install Cilium

```bash
# Add Cilium Helm repository
helm repo add cilium https://helm.cilium.io/
helm repo update

# Install Cilium with OCI IPAM
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values cilium-oci-values.yaml

# Verify installation
kubectl -n kube-system rollout status deployment/cilium-operator
kubectl -n kube-system rollout status daemonset/cilium
```

## Step 4: Verify Installation

### Check Cilium Operator Logs

```bash
# Check operator logs for OCI initialization
kubectl -n kube-system logs deployment/cilium-operator | grep -i oci

# Expected output should include:
# level=info msg="Initializing OCI client ..."
# level=info msg="Using instance principal authentication for OCI"
# level=info msg="Init AlloctorOCI successful"
# level=info msg="Starting OCI VNIC allocator..."
```

### Check CiliumNode Status

```bash
# List CiliumNodes
kubectl get ciliumnodes

# Check detailed status of a node
kubectl get ciliumnode <node-name> -o yaml

# Verify OCI IPAM status
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci}' | jq
```

Expected output:
```json
{
  "vnics": {
    "ocid1.vnic.oc1.phx.xxx": {
      "id": "ocid1.vnic.oc1.phx.xxx",
      "mac": "02:00:17:xx:xx:xx",
      "primary-ip": "10.0.1.5",
      "addresses": [
        "10.0.1.5",
        "10.0.1.10",
        "10.0.1.11"
      ],
      "subnet": {
        "id": "ocid1.subnet.oc1.phx.xxx",
        "cidr": "10.0.1.0/24"
      },
      "vcn": {
        "id": "ocid1.vcn.oc1.phx.xxx",
        "cidr-blocks": ["10.0.0.0/16"]
      }
    }
  }
}
```

### Test Pod Connectivity

```bash
# Create a test deployment
kubectl create deployment nginx --image=nginx --replicas=3

# Check pod IPs (should be from OCI VCN subnet)
kubectl get pods -o wide

# Test connectivity between pods
kubectl exec -it <pod-name> -- ping <another-pod-ip>

# Test connectivity to OCI resources
kubectl exec -it <pod-name> -- ping <oci-instance-private-ip>
```

## Step 5: Configure Subnet Tags (Optional)

You can use subnet tags to control which subnets Cilium uses for IP allocation:

1. **Tag your subnets in OCI Console**:
   ```
   Key: cilium-pool
   Value: production
   ```

2. **Update your CiliumNode spec**:
   ```yaml
   apiVersion: cilium.io/v2
   kind: CiliumNode
   metadata:
     name: worker-node-1
   spec:
     oci:
       vcn-id: ocid1.vcn.oc1.phx.xxx
       availability-domain: "AD-1"
       subnet-tags:
         cilium-pool: production
   ```

## Troubleshooting

### Issue: Operator fails with "OCI VCN ID is required"

**Solution**: Ensure you've set the `--oci-vcn-id` flag in operator args:

```yaml
operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
```

### Issue: "failed to create instance principal config provider"

**Possible Causes**:
1. Instance is not in the dynamic group
2. IAM policies are missing or incorrect
3. Instance doesn't have proper tags

**Solution**: 
- Verify dynamic group membership
- Check IAM policies
- Use config file auth instead: `OCIUseInstancePrincipal: false`

### Issue: "unable to find matching subnet"

**Possible Causes**:
1. No subnets in the specified VCN
2. All subnets are full
3. Subnet tags don't match
4. Availability domain mismatch

**Solution**:
```bash
# Check available subnets
oci network subnet list --vcn-id <vcn-ocid> --compartment-id <compartment-ocid>

# Verify subnet has available IPs
# Each subnet reserves 3 IPs for OCI
```

### Issue: Pods not getting IPs

**Check**:
```bash
# 1. Check operator logs
kubectl -n kube-system logs deployment/cilium-operator | grep -i error

# 2. Check CiliumNode IPAM status
kubectl get ciliumnode <node-name> -o jsonpath='{.status.ipam}' | jq

# 3. Check VNIC attachments in OCI Console
# Navigate to: Compute → Instances → <instance> → Attached VNICs

# 4. Verify operator has proper permissions
kubectl -n kube-system logs deployment/cilium-operator | grep -i "unauthorized\|forbidden"
```

### Issue: High VNIC creation rate

**Solution**: Adjust pre-allocation settings:

```yaml
operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
    - --nodes-ipam-pod-cidr-allocation-threshold=10
    - --nodes-ipam-pod-cidr-release-threshold=5
```

## Advanced Configuration

### Configure IP Pre-allocation

```yaml
ipam:
  mode: "oci"
  operator:
    # Pre-allocate IPs for faster pod startup
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"
    clusterPoolIPv4MaskSize: 24
```

### Enable Metrics

```yaml
operator:
  prometheus:
    enabled: true
    serviceMonitor:
      enabled: true
```

### Configure Resource Limits

```yaml
operator:
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 100m
      memory: 128Mi
```

## Limitations

1. **VCN CIDR**: Pods get IPs from VCN subnets, so ensure your VCN CIDR is large enough
2. **VNIC Limits**: Each instance shape has a maximum number of VNICs
3. **IPs per VNIC**: Currently limited to 32 IPs per VNIC
4. **Regional Subnets**: Availability domain filtering is optional (supports regional subnets)

## Shape-Specific VNIC Limits

Common OCI shapes and their VNIC limits:

| Shape | Max VNICs | IPs per VNIC | Total IPs |
|-------|-----------|--------------|-----------|
| VM.Standard.E4.Flex (1-8 OCPUs) | 2 | 32 | 64 |
| VM.Standard.E4.Flex (9+ OCPUs) | varies | 32 | varies |
| VM.Standard3.Flex | varies | 32 | varies |
| BM.Standard.E4.128 | 24 | 32 | 768 |

**Note**: Flex shapes have dynamic limits based on OCPU count. Cilium queries the instance-level limit automatically.

## Next Steps

- [Configure Network Policies](../policy/)
- [Enable Hubble Observability](../../observability/hubble/)
- [Set up Service Mesh](../../servicemesh/)
- [Monitor IPAM metrics](../../operations/metrics/)

## References

- [OCI VCN Documentation](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/managingVCNs_topic-Overview_of_VCNs_and_Subnets.htm)
- [OCI Instance Principals](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm)
- [OCI Compute Shapes](https://docs.oracle.com/en-us/iaas/Content/Compute/References/computeshapes.htm)
- [Cilium IPAM Concepts](https://docs.cilium.io/en/stable/network/concepts/ipam/)

## Support

For issues specific to OCI IPAM:
1. Check operator logs: `kubectl -n kube-system logs deployment/cilium-operator`
2. Verify IAM permissions in OCI Console
3. Review CiliumNode status: `kubectl get ciliumnodes -o yaml`
4. Open an issue on GitHub with logs and configuration
