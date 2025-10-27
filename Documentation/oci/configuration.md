# Cilium OCI IPAM Configuration Reference

Complete reference for all OCI IPAM configuration options.

**Version**: Cilium v1.15.2  
**Last Updated**: October 27, 2025

---

## Table of Contents

- [IPAM Configuration](#ipam-configuration)
- [OCI Configuration](#oci-configuration)
- [Operator Configuration](#operator-configuration)
- [Advanced Configuration](#advanced-configuration)
- [Configuration Examples](#configuration-examples)

---

## IPAM Configuration

### ipam.mode

**Type**: `string`  
**Required**: Yes  
**Default**: `"cluster-pool"`

**Description**: Set to `"oci"` to enable OCI IPAM mode.

```yaml
ipam:
  mode: oci
```

### ipam.operator.clusterPoolIPv4PodCIDRList

**Type**: `[]string`  
**Required**: Yes for OCI mode  
**Default**: `[]`

**Description**: List of CIDRs that match your VCN CIDR. Used for route propagation.

```yaml
ipam:
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"  # Must match your VCN CIDR
```

**Best Practice**: Use your VCN's CIDR block, not individual subnet CIDRs.

---

## OCI Configuration

### oci.enabled

**Type**: `boolean`  
**Required**: Yes  
**Default**: `false`

**Description**: Enable OCI-specific features.

```yaml
oci:
  enabled: true
```

### oci.useInstancePrincipal

**Type**: `boolean`  
**Required**: Yes  
**Default**: `false`

**Description**: Use Instance Principal for OCI API authentication.

```yaml
oci:
  useInstancePrincipal: true
```

**Recommended**: `true` for production environments (more secure).

**Alternative**: Set to `false` and configure API key authentication (not recommended).

### oci.vcnID

**Type**: `string`  
**Required**: Yes  
**Default**: `""`

**Description**: OCID of the VCN where your Kubernetes cluster runs.

```yaml
oci:
  vcnID: "ocid1.vcn.oc1.ap-singapore-2.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"
```

**How to find**: OCI Console → Networking → Virtual Cloud Networks → Copy OCID

### oci.subnetTags

**Type**: `map[string]string`  
**Required**: No  
**Default**: `{}`

**Description**: Freeform tags to match subnets for automatic VNIC creation.

```yaml
oci:
  subnetTags:
    cilium-pod-network: "yes"
    environment: "production"
```

**Important**: This alone is NOT enough. You must also configure `operator.extraArgs` (see below).

**Use Case**: Enable automatic VNIC creation from tagged subnets.

### oci.vnicPreAllocationThreshold

**Type**: `float`  
**Required**: No  
**Default**: `0.8`

**Description**: Threshold for pre-allocating VNICs. When IP usage reaches this percentage, a new VNIC is created.

```yaml
oci:
  vnicPreAllocationThreshold: 0.8  # 80%
```

**Range**: `0.0` to `1.0`

**Tuning**:
- Lower value (e.g., `0.7`): More proactive, may create unnecessary VNICs
- Higher value (e.g., `0.9`): More conservative, risk of Pod creation delays

### oci.maxIPsPerVNIC

**Type**: `integer`  
**Required**: No  
**Default**: `32`

**Description**: Maximum number of secondary IPs per VNIC.

```yaml
oci:
  maxIPsPerVNIC: 32
```

**Note**: OCI limit is 32 secondary IPs per VNIC. Do not change unless OCI increases this limit.

### oci.ipAllocationTimeout

**Type**: `integer`  
**Required**: No  
**Default**: `60`

**Description**: Timeout in seconds for IP allocation operations.

```yaml
oci:
  ipAllocationTimeout: 60
```

**Tuning**:
- Increase if you see timeout errors
- Decrease for faster failure detection

### oci.maxVNICsPerNode

**Type**: `integer`  
**Required**: No  
**Default**: Auto-detected from instance shape

**Description**: Maximum number of VNICs per node. Overrides auto-detection.

```yaml
oci:
  maxVNICsPerNode: 8
```

**Warning**: Setting this incorrectly can cause issues. Only use if auto-detection fails.

**Instance Shape Limits**:
- VM.Standard.E5.Flex: 2-8 VNICs
- VM.Standard3.Flex: 2-8 VNICs
- BM.Standard.E5.192: 24 VNICs

---

## Operator Configuration

### operator.replicas

**Type**: `integer`  
**Required**: No  
**Default**: `1`

**Description**: Number of Operator replicas for high availability.

```yaml
operator:
  replicas: 2
```

**Recommended**: `2` for production environments.

### operator.extraArgs

**Type**: `[]string`  
**Required**: **YES if using Subnet Tags**  
**Default**: `[]`

**Description**: Additional command-line arguments for the Operator.

**Critical for Subnet Tags**:

```yaml
operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.ap-singapore-2.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq
    - --oci-use-instance-principal=true
    - --subnet-tags-filter=cilium-pod-network=yes
```

**Why needed**: The Operator doesn't automatically read `oci.subnetTags` from Helm values. You must explicitly pass `--subnet-tags-filter`.

**Format**: `--subnet-tags-filter=<key>=<value>`

### operator.resources

**Type**: `object`  
**Required**: No  
**Default**: See below

**Description**: Resource requests and limits for the Operator.

```yaml
operator:
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 1Gi
```

**Tuning**:
- Increase for large clusters (100+ nodes)
- Decrease for small test clusters

---

## Advanced Configuration

### Debug Logging

Enable debug logging for troubleshooting:

```yaml
operator:
  extraArgs:
    - --debug
    - --debug-verbose=datapath,flow

agent:
  debug:
    enabled: true
```

### Custom VNIC Naming

Not currently configurable. VNICs are named automatically.

### Network Security Groups

Apply Network Security Groups to VNICs:

```yaml
oci:
  networkSecurityGroupIDs:
    - "ocid1.networksecuritygroup.oc1.region.aaaaa..."
    - "ocid1.networksecuritygroup.oc1.region.bbbbb..."
```

**Use Case**: Apply OCI NSG rules to all Pod traffic.

---

## Configuration Examples

### Example 1: Minimal Configuration

```yaml
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"
```

### Example 2: With Subnet Tags (Recommended)

```yaml
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"
  subnetTags:
    cilium-pod-network: "yes"
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32

operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq
    - --oci-use-instance-principal=true
    - --subnet-tags-filter=cilium-pod-network=yes
```

### Example 3: Production Configuration

```yaml
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"
  subnetTags:
    cilium-pod-network: "yes"
    environment: "production"
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32
  ipAllocationTimeout: 60

operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq
    - --oci-use-instance-principal=true
    - --subnet-tags-filter=cilium-pod-network=yes
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 1Gi
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              io.cilium/app: operator
          topologyKey: kubernetes.io/hostname

agent:
  resources:
    requests:
      cpu: 250m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi

prometheus:
  enabled: true
  serviceMonitor:
    enabled: true

hubble:
  enabled: true
  metrics:
    enabled:
      - dns:query;ignoreAAAA
      - drop
      - tcp
      - flow
      - icmp
      - http
  relay:
    enabled: true
    replicas: 2
  ui:
    enabled: true
```

### Example 4: Development/Testing

```yaml
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"

operator:
  replicas: 1
  extraArgs:
    - --debug
    - --oci-vcn-id=ocid1.vcn.oc1.region.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq
    - --oci-use-instance-principal=true

agent:
  debug:
    enabled: true
```

---

## Validation

### Verify Configuration

After deploying, verify your configuration:

```bash
# Check Helm values
helm get values cilium -n kube-system

# Check if Subnet Tags is configured correctly
kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter

# Should see: --subnet-tags-filter='your-tag=value'
# If empty: --subnet-tags-filter='' then operator.extraArgs is missing
```

### Common Mistakes

❌ **Mistake 1**: Only configured `oci.subnetTags`, forgot `operator.extraArgs`

```yaml
# WRONG - Subnet Tags won't work
oci:
  subnetTags:
    key: "value"
# Missing operator.extraArgs!
```

✅ **Correct**:

```yaml
oci:
  subnetTags:
    key: "value"

operator:
  extraArgs:
    - --subnet-tags-filter=key=value
```

❌ **Mistake 2**: Wrong CIDR in `clusterPoolIPv4PodCIDRList`

```yaml
# WRONG - Using subnet CIDR instead of VCN CIDR
ipam:
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.1.0/24"  # Subnet CIDR
```

✅ **Correct**:

```yaml
ipam:
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"  # VCN CIDR
```

---

## Upgrading Configuration

### Add Subnet Tags to Existing Deployment

```bash
helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  --reuse-values \
  --set oci.subnetTags.cilium-pod-network="yes" \
  --set-string 'operator.extraArgs={--oci-vcn-id=ocid1.vcn...,--oci-use-instance-principal=true,--subnet-tags-filter=cilium-pod-network=yes}'
```

**Note**: Use `--set-string` for arrays to avoid parsing issues.

### Change VNIC Threshold

```bash
helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  --reuse-values \
  --set oci.vnicPreAllocationThreshold=0.7
```

---

## Troubleshooting Configuration

### Issue: Configuration Not Applied

**Check**:
```bash
# View actual pod configuration
kubectl get configmap -n kube-system cilium-config -o yaml

# Check Operator args
kubectl get deployment -n kube-system cilium-operator -o yaml | grep args -A 20
```

### Issue: Helm Values Not Taking Effect

**Solution**: Some values require pod restart:

```bash
kubectl rollout restart deployment/cilium-operator -n kube-system
kubectl rollout restart daemonset/cilium -n kube-system
```

---

## References

- [quickstart.md](quickstart.md) - Quick start guide
- [troubleshooting.md](troubleshooting.md) - Troubleshooting guide
- [Cilium Helm Reference](https://docs.cilium.io/en/stable/helm-reference/)
- [OCI API Documentation](https://docs.oracle.com/en-us/iaas/api/)

---

**Version**: 1.0  
**Last Updated**: October 27, 2025  
**Maintainer**: Dengwei (SEHUB)
