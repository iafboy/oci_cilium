# OCI IPAM Configuration Reference

Complete reference for all configuration options available for Cilium OCI IPAM.

## Table of Contents

- [Operator Flags](#operator-flags)
- [Helm Chart Values](#helm-chart-values)
- [CiliumNode Spec](#ciliumnode-spec)
- [Environment Variables](#environment-variables)
- [Advanced Configuration](#advanced-configuration)
- [Examples](#examples)

---

## Operator Flags

### Required Flags

#### `--oci-vcn-id`
**Type:** String  
**Required:** Yes  
**Default:** None

The OCID of the OCI VCN (Virtual Cloud Network) where IP addresses will be allocated.

```bash
--oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaaaaa...
```

**How to find:**
```bash
# Using OCI CLI
oci network vcn list --compartment-id <compartment-ocid>

# Or in OCI Console
# Navigate to: Networking → Virtual Cloud Networks
```

### Optional Flags

#### `--oci-use-instance-principal`
**Type:** Boolean  
**Required:** No  
**Default:** `true`

Use OCI Instance Principal authentication instead of config file.

```bash
--oci-use-instance-principal=true   # Use instance principals (recommended)
--oci-use-instance-principal=false  # Use ~/.oci/config file
```

**When to use false:**
- Development/testing environments
- When instance principals aren't configured
- When you need user-specific permissions

---

#### `--parallel-alloc-workers`
**Type:** Integer  
**Required:** No  
**Default:** `50`

Number of parallel workers for IP allocation.

```bash
--parallel-alloc-workers=10  # Lower for small clusters
--parallel-alloc-workers=100 # Higher for large clusters
```

**Recommendations:**
- Small clusters (<50 nodes): 10-20
- Medium clusters (50-200 nodes): 50
- Large clusters (>200 nodes): 100+

---

#### `--nodes-ipam-pod-cidr-allocation-threshold`
**Type:** Integer  
**Required:** No  
**Default:** `0`

Number of IPs to pre-allocate on each node.

```bash
--nodes-ipam-pod-cidr-allocation-threshold=10
```

**Impact:**
- Higher values = faster pod starts, but more IP waste
- Lower values = slower pod starts, but better IP utilization
- `0` = allocate on-demand only

---

#### `--nodes-ipam-pod-cidr-release-threshold`
**Type:** Integer  
**Required:** No  
**Default:** `0`

Number of free IPs to maintain before releasing VNIC.

```bash
--nodes-ipam-pod-cidr-release-threshold=5
```

**Impact:**
- Prevents frequent VNIC churn
- Set to 0 to release VNICs immediately when unused

---

#### `--nodes-ipam-sync-interval`
**Type:** Duration  
**Required:** No  
**Default:** `30s`

Interval between IPAM reconciliation cycles.

```bash
--nodes-ipam-sync-interval=60s   # Less frequent (lower CPU)
--nodes-ipam-sync-interval=15s   # More frequent (faster recovery)
```

---

## Helm Chart Values

### Top-Level Configuration

```yaml
# IPAM mode (required)
ipam:
  mode: "oci"  # Must be set to "oci"
  
# OCI-specific configuration
oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"  # Your VCN OCID

# Authentication method
OCIUseInstancePrincipal: true
```

### Operator Configuration

```yaml
operator:
  # Number of operator replicas
  replicas: 2
  
  # Additional command-line arguments
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
    - --parallel-alloc-workers=50
    - --nodes-ipam-pod-cidr-allocation-threshold=10
    - --nodes-ipam-pod-cidr-release-threshold=5
  
  # Resource limits
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 100m
      memory: 128Mi
  
  # Node affinity (optional)
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""
  
  # Tolerations (optional)
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
  
  # Prometheus metrics
  prometheus:
    enabled: true
    port: 9963
    serviceMonitor:
      enabled: true
  
  # Update strategy
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 0
```

### Agent Configuration

```yaml
agent:
  # Enable agent on each node
  enabled: true
  
  # Resource limits for agent
  resources:
    limits:
      cpu: 4000m
      memory: 4Gi
    requests:
      cpu: 100m
      memory: 512Mi
```

### Kubernetes Integration

```yaml
# Kubernetes API server configuration
k8sServiceHost: "10.0.0.1"  # Your K8s API server IP
k8sServicePort: "6443"

# Service accounts
serviceAccounts:
  operator:
    create: true
    name: cilium-operator
    automount: true
```

### Network Configuration

```yaml
# Tunnel mode (recommended for OCI)
tunnelProtocol: "vxlan"  # or "geneve"
routingMode: "tunnel"

# IPv4/IPv6 configuration
enableIPv4: true
enableIPv6: false

# Masquerading
enableIPv4Masquerade: true
enableIPv6Masquerade: false

# MTU configuration
mtu: 1500  # Adjust based on your network
```

### Debug and Logging

```yaml
debug:
  enabled: false  # Set to true for verbose logging
  verbose: ""     # Specific subsystems: datapath, flow, kvstore

# Log format
logSystemLoad: false
```

---

## CiliumNode Spec

### Basic Configuration

```yaml
apiVersion: cilium.io/v2
kind: CiliumNode
metadata:
  name: node-1
  namespace: kube-system
spec:
  # OCI-specific configuration
  oci:
    # VCN OCID (usually inherited from operator config)
    vcn-id: "ocid1.vcn.oc1.phx.xxx"
    
    # Availability domain (optional)
    availability-domain: "AD-1"
    
    # Instance type/shape
    instance-type: "VM.Standard.E4.Flex"
    
    # Subnet selection tags (optional)
    subnet-tags:
      environment: "production"
      tier: "backend"
```

### Status Fields

```yaml
status:
  oci:
    # Attached VNICs
    vnics:
      "ocid1.vnic.oc1.phx.vnic1":
        id: "ocid1.vnic.oc1.phx.vnic1"
        mac: "02:00:17:xx:xx:xx"
        primary-ip: "10.0.1.5"
        is-primary: true
        addresses:
          - "10.0.1.5"
          - "10.0.1.10"
          - "10.0.1.11"
        subnet:
          id: "ocid1.subnet.oc1.phx.subnet1"
          cidr: "10.0.1.0/24"
        vcn:
          id: "ocid1.vcn.oc1.phx.vcn1"
          cidr-blocks:
            - "10.0.0.0/16"
  
  # IPAM status
  ipam:
    operator-status:
      error: ""
    used:
      "10.0.1.10": "default/pod-1"
      "10.0.1.11": "default/pod-2"
    available:
      "10.0.1.12": {}
      "10.0.1.13": {}
```

---

## Environment Variables

### Operator Pod Environment

```yaml
env:
  # OCI Authentication
  - name: OCI_CLI_AUTH
    value: "instance_principal"  # or "api_key"
  
  # Kubernetes node name
  - name: K8S_NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
  
  # Cilium namespace
  - name: CILIUM_K8S_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
  
  # Debug mode
  - name: CILIUM_DEBUG
    value: "false"
  
  # Kubernetes API
  - name: KUBERNETES_SERVICE_HOST
    value: "10.0.0.1"
  - name: KUBERNETES_SERVICE_PORT
    value: "6443"
```

### OCI Config File (when not using Instance Principals)

```yaml
# Mount OCI config as secret
volumes:
  - name: oci-config
    secret:
      secretName: oci-config
      items:
        - key: config
          path: config
        - key: api-key
          path: api-key.pem

volumeMounts:
  - name: oci-config
    mountPath: /root/.oci
    readOnly: true
```

---

## Advanced Configuration

### Subnet Selection Strategy

```yaml
# In CiliumNode spec
spec:
  oci:
    vcn-id: "ocid1.vcn.oc1.phx.xxx"
    
    # Priority order for subnet selection:
    # 1. Subnets in specified availability domain
    availability-domain: "AD-1"
    
    # 2. Subnets with matching tags
    subnet-tags:
      cilium-pool: "production"
      tier: "backend"
    
    # 3. Subnet with most available IPs (default behavior)
```

### VNIC Configuration

```yaml
# These are controlled by OCI shape and cannot be directly configured
# But you can influence them:

# Maximum VNICs per node: Determined by instance shape
# - Small shapes (2-4 OCPUs): 2 VNICs
# - Medium shapes (8-16 OCPUs): 4-8 VNICs
# - Large shapes (32+ OCPUs): 8-24 VNICs

# IPs per VNIC: Fixed at 32 secondary IPs + 1 primary

# To maximize capacity:
# 1. Use larger instance shapes
# 2. Use Flex shapes with more OCPUs
# 3. Enable pre-allocation to use VNICs efficiently
```

### High Availability Configuration

```yaml
operator:
  # Multiple replicas for HA
  replicas: 3
  
  # Anti-affinity to spread across nodes
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              io.cilium/app: operator
          topologyKey: kubernetes.io/hostname
  
  # Topology spread
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          io.cilium/app: operator
```

### Monitoring and Metrics

```yaml
# Enable Prometheus metrics
operator:
  prometheus:
    enabled: true
    port: 9963
    serviceMonitor:
      enabled: true
      interval: "10s"
      labels:
        prometheus: kube-prometheus

# Hubble for observability
hubble:
  enabled: true
  metrics:
    enabled:
      - dns
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

### Performance Tuning

```yaml
operator:
  extraArgs:
    # Increase workers for large clusters
    - --parallel-alloc-workers=100
    
    # Faster reconciliation
    - --nodes-ipam-sync-interval=15s
    
    # Aggressive pre-allocation
    - --nodes-ipam-pod-cidr-allocation-threshold=20
    
    # Conservative release
    - --nodes-ipam-pod-cidr-release-threshold=10
  
  # More resources for operator
  resources:
    limits:
      cpu: 2000m
      memory: 2Gi
    requests:
      cpu: 500m
      memory: 512Mi
```

---

## Examples

### Minimal Production Configuration

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"

OCIUseInstancePrincipal: true

operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 100m
      memory: 128Mi

k8sServiceHost: "10.0.0.1"
k8sServicePort: "6443"

enableIPv4Masquerade: true
tunnelProtocol: "vxlan"
```

### High-Performance Configuration

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"

OCIUseInstancePrincipal: true

operator:
  replicas: 3
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
    - --parallel-alloc-workers=100
    - --nodes-ipam-pod-cidr-allocation-threshold=20
    - --nodes-ipam-pod-cidr-release-threshold=10
    - --nodes-ipam-sync-interval=15s
  
  resources:
    limits:
      cpu: 2000m
      memory: 2Gi
    requests:
      cpu: 500m
      memory: 512Mi
  
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                io.cilium/app: operator
            topologyKey: kubernetes.io/hostname

k8sServiceHost: "10.0.0.1"
k8sServicePort: "6443"

enableIPv4Masquerade: true
tunnelProtocol: "vxlan"
routingMode: "tunnel"

# Enable metrics and observability
operator:
  prometheus:
    enabled: true
    serviceMonitor:
      enabled: true

hubble:
  enabled: true
  metrics:
    enabled:
      - dns
      - drop
      - tcp
      - flow
  relay:
    enabled: true
  ui:
    enabled: true
```

### Development/Testing Configuration

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"

# Use config file auth for testing
OCIUseInstancePrincipal: false

operator:
  replicas: 1
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
    - --parallel-alloc-workers=10
  
  resources:
    limits:
      cpu: 500m
      memory: 512Mi
    requests:
      cpu: 100m
      memory: 128Mi

# Enable debug logging
debug:
  enabled: true
  verbose: "datapath"

k8sServiceHost: "10.0.0.1"
k8sServicePort: "6443"
```

### Multi-Region Configuration

```yaml
# Region 1 (Phoenix)
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"  # Phoenix VCN

OCIUseInstancePrincipal: true

operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
  
  # Deploy operator in specific region
  nodeSelector:
    topology.kubernetes.io/region: "phx"

---

# Region 2 (Ashburn) - separate cluster
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.iad.xxx"  # Ashburn VCN

OCIUseInstancePrincipal: true

operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.iad.xxx
  
  nodeSelector:
    topology.kubernetes.io/region: "iad"
```

### With Subnet Tags

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"

OCIUseInstancePrincipal: true

operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx

# Then create CiliumNodes with specific subnet requirements
---
apiVersion: cilium.io/v2
kind: CiliumNode
metadata:
  name: worker-1
spec:
  oci:
    vcn-id: "ocid1.vcn.oc1.phx.xxx"
    availability-domain: "AD-1"
    subnet-tags:
      environment: "production"
      tier: "frontend"

---
apiVersion: cilium.io/v2
kind: CiliumNode
metadata:
  name: worker-2
spec:
  oci:
    vcn-id: "ocid1.vcn.oc1.phx.xxx"
    availability-domain: "AD-2"
    subnet-tags:
      environment: "production"
      tier: "backend"
```

---

## Configuration Validation

### Pre-Deployment Checklist

```bash
# 1. Verify VCN exists and is accessible
oci network vcn get --vcn-id <your-vcn-id>

# 2. Check compartment ID
oci iam compartment list

# 3. Verify IAM policies
oci iam policy list --compartment-id <compartment-id>

# 4. List available subnets
oci network subnet list --vcn-id <your-vcn-id>

# 5. Check subnet capacity
oci network subnet get --subnet-id <subnet-id> | \
  jq '.data."available-ip-address-count"'

# 6. Verify instance shapes
oci compute shape list --compartment-id <compartment-id> | \
  jq '.data[] | select(.shape | startswith("VM.")) | {shape, vnics: ."max-vnic-attachments"}'
```

### Post-Deployment Verification

```bash
# 1. Check operator status
kubectl -n kube-system get deployment cilium-operator

# 2. View operator logs
kubectl -n kube-system logs deployment/cilium-operator | grep -i oci

# 3. Verify CiliumNodes
kubectl get ciliumnodes

# 4. Check IPAM status
kubectl get ciliumnode <node-name> -o jsonpath='{.status.ipam}' | jq

# 5. Test pod creation
kubectl create deployment test --image=nginx --replicas=3
kubectl get pods -o wide
```

---

## Configuration Best Practices

1. **Always use Instance Principals in production** - More secure than config files
2. **Set appropriate pre-allocation thresholds** - Balance between speed and IP waste
3. **Monitor VNIC usage** - Set up alerts before limits are reached
4. **Use multiple subnets** - Distribute load and increase availability
5. **Right-size operator resources** - Adjust based on cluster size
6. **Enable metrics** - Monitor IPAM performance
7. **Use HA for operator** - At least 2 replicas
8. **Document custom configurations** - Maintain GitOps repository
9. **Test in dev first** - Validate all settings before production
10. **Regular reviews** - Audit configuration quarterly

---

## Related Documentation

- [Quick Start Guide](quickstart.md)
- [Troubleshooting Guide](troubleshooting.md)
- [OCI IPAM Overview](README.md)
- [Cilium IPAM Concepts](https://docs.cilium.io/en/stable/network/concepts/ipam/)

---

## Configuration Support

For questions about specific configuration options:

1. Check operator logs for validation errors
2. Review [troubleshooting guide](troubleshooting.md)
3. Consult [OCI documentation](https://docs.oracle.com/en-us/iaas/Content/Network/home.htm)
4. Ask in [Cilium Slack](https://cilium.io/slack)
5. Open a GitHub issue with your configuration
