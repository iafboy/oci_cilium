# Cilium OCI IPAM Quick Start Guide

This guide will help you deploy Cilium with OCI IPAM in 30 minutes.

**Prerequisites**: 
- Kubernetes cluster running on OCI
- kubectl configured
- Helm 3.x installed

---

## Step 1: Prepare OCI Environment

### 1.1 Gather Information

Collect these OCIDs:

```bash
# VCN OCID
export VCN_OCID="ocid1.vcn.oc1.region.amaaaaaa..."

# Compartment OCID
export COMPARTMENT_OCID="ocid1.compartment.oc1..aaaaaaaaa..."

# Subnets for Pods (at least one)
export POD_SUBNET_1="ocid1.subnet.oc1.region.aaaaaaaaa..."
export POD_SUBNET_2="ocid1.subnet.oc1.region.aaaaaaaaa..."
```

### 1.2 Configure IAM (Critical!)

#### Create Dynamic Group

Go to OCI Console → Identity → Dynamic Groups → Create

**Name**: `cilium-oci-ipam`

**Matching Rules**:
```
instance.compartment.id = 'ocid1.compartment.oc1..aaaaaaaa...'
```

#### Create Policy

Go to OCI Console → Identity → Policies → Create Policy

**Name**: `cilium-oci-ipam-policy`

**Statements**:
```
Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use subnets in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use private-ips in compartment <compartment-name>
```

#### Verify Permissions

SSH to any K8s node and run:

```bash
oci iam region list --auth instance_principal
```

✅ **Success**: You see a list of regions  
❌ **Failure**: "Unauthorized" or "Forbidden" error → Check Dynamic Group and Policy

---

## Step 2: Choose Deployment Method

### Option A: With Subnet Tags (Recommended) ⭐

**Advantages**:
- Fully automated VNIC creation
- No manual VNIC management
- Flexible subnet selection

**Use when**: You want automatic VNIC management

### Option B: Manual VNIC Creation

**Advantages**:
- More control over VNICs
- Works without subnet tags

**Use when**: You prefer manual control

---

## Step 3A: Deploy with Subnet Tags (Recommended)

### 3A.1 Tag Your Subnets

```bash
# Add freeform tag to Pod subnets
for subnet in $POD_SUBNET_1 $POD_SUBNET_2; do
  oci network subnet update \
    --subnet-id $subnet \
    --freeform-tags '{"cilium-pod-network":"yes"}' \
    --force \
    --auth instance_principal
done
```

Verify:
```bash
oci network subnet get \
  --subnet-id $POD_SUBNET_1 \
  --query 'data."freeform-tags"' \
  --auth instance_principal

# Should see: {"cilium-pod-network": "yes"}
```

### 3A.2 Create Helm Values File

```bash
cat > cilium-oci-values.yaml <<EOF
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"  # Replace with your VCN CIDR

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "$VCN_OCID"
  
  # Subnet Tags configuration (1/2)
  subnetTags:
    cilium-pod-network: "yes"
  
  # VNIC management
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32

operator:
  replicas: 2
  
  # ⚠️ CRITICAL: Must configure extraArgs (2/2)
  extraArgs:
    - --oci-vcn-id=$VCN_OCID
    - --oci-use-instance-principal=true
    - --subnet-tags-filter=cilium-pod-network=yes

# Monitoring (optional but recommended)
prometheus:
  enabled: true

hubble:
  enabled: true
  relay:
    enabled: true
EOF
```

⚠️ **Important**: Both `oci.subnetTags` AND `operator.extraArgs[--subnet-tags-filter]` are required!

### 3A.3 Install Cilium

```bash
helm repo add cilium https://helm.cilium.io/
helm repo update

helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values cilium-oci-values.yaml
```

### 3A.4 Verify Installation

```bash
# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l k8s-app=cilium -n kube-system --timeout=300s

# Check Cilium status
cilium status

# Verify Subnet Tags configuration
kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter

# Should see: --subnet-tags-filter='cilium-pod-network=yes'
```

✅ **Success**: Cilium pods are Running and subnet-tags-filter shows the correct value

---

## Step 3B: Deploy with Manual VNIC Creation

### 3B.1 Create Additional VNICs (Optional)

If you want more than the primary VNIC:

```bash
# For each worker node
oci compute vnic-attachment attach \
  --instance-id ocid1.instance.oc1.region.anzxsljr... \
  --subnet-id $POD_SUBNET_1 \
  --display-name "cilium-vnic-2" \
  --auth instance_principal
```

### 3B.2 Create Helm Values File

```bash
cat > cilium-oci-values.yaml <<EOF
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"  # Replace with your VCN CIDR

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "$VCN_OCID"
  
  # VNIC management
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32

operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=$VCN_OCID
    - --oci-use-instance-principal=true

prometheus:
  enabled: true

hubble:
  enabled: true
  relay:
    enabled: true
EOF
```

### 3B.3 Install Cilium

Same as Step 3A.3

---

## Step 4: Verify Deployment

### 4.1 Check Cilium Pods

```bash
kubectl get pods -n kube-system -l k8s-app=cilium

# Expected: All pods Running
```

### 4.2 Check CiliumNode

```bash
kubectl get ciliumnode

# Should see all your nodes
```

View VNIC details:

```bash
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq

# Should see VNIC information with IPs
```

### 4.3 Create Test Pods

```bash
kubectl create deployment test-nginx --image=nginx --replicas=3

kubectl get pods -o wide

# Verify Pods get IPs from VCN subnets (10.0.x.x range)
```

### 4.4 Test Network Connectivity

```bash
# Get test pod names
POD1=$(kubectl get pods -l app=test-nginx -o jsonpath='{.items[0].metadata.name}')
POD2=$(kubectl get pods -l app=test-nginx -o jsonpath='{.items[1].metadata.name}')
POD2_IP=$(kubectl get pod $POD2 -o jsonpath='{.status.podIP}')

# Test Pod to Pod
kubectl exec $POD1 -- ping -c 3 $POD2_IP

# Test Pod to Internet
kubectl exec $POD1 -- ping -c 3 8.8.8.8

# Test DNS
kubectl exec $POD1 -- nslookup kubernetes.default
```

✅ **All tests pass**: Your OCI IPAM is working correctly!

---

## Step 5: Enable Monitoring (Optional)

### 5.1 Verify Prometheus Metrics

```bash
kubectl port-forward -n kube-system svc/hubble-metrics 9965:9965

# In another terminal
curl http://localhost:9965/metrics | grep cilium_oci
```

### 5.2 Access Hubble UI

```bash
kubectl port-forward -n kube-system svc/hubble-ui 12000:80

# Open browser: http://localhost:12000
```

---

## Common Issues

### Issue 1: Pods Stuck in ContainerCreating

**Check**:
```bash
kubectl describe pod <pod-name>
kubectl get ciliumnode
kubectl logs -n kube-system deployment/cilium-operator
```

**Common causes**:
- IAM permissions not configured
- Subnet IP exhaustion
- VNIC limit reached

### Issue 2: Subnet Tags Not Working

**Symptom**: Operator log shows `--subnet-tags-filter=''`

**Solution**: You forgot `operator.extraArgs`. Add:

```yaml
operator:
  extraArgs:
    - --subnet-tags-filter=cilium-pod-network=yes
```

### Issue 3: IAM Permission Errors

**Symptom**: "Unauthorized" or "Forbidden" in logs

**Solution**: 
1. Verify Dynamic Group includes your instances
2. Verify Policy statements are correct
3. Test: `oci iam region list --auth instance_principal`

---

## Next Steps

### Production Checklist

- [ ] Configure monitoring alerts
- [ ] Set up log aggregation
- [ ] Enable Hubble for observability
- [ ] Plan subnet capacity (/24 or larger)
- [ ] Test failover scenarios
- [ ] Document your deployment

### Learn More

- [configuration.md](configuration.md) - All configuration options
- [troubleshooting.md](troubleshooting.md) - Detailed troubleshooting
- `CILIUM_OCI_IPAM_DEPLOYMENT_MANUAL.md` - Complete deployment manual

---

## Quick Reference

### Useful Commands

```bash
# Cilium status
cilium status

# List CiliumNodes
kubectl get ciliumnode

# View VNIC details
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq

# Operator logs
kubectl logs -n kube-system deployment/cilium-operator --tail=100

# Agent logs
kubectl logs -n kube-system daemonset/cilium -c cilium-agent --tail=100

# Test IAM permissions
oci iam region list --auth instance_principal
```

### Helm Operations

```bash
# View current config
helm get values cilium -n kube-system

# Upgrade configuration
helm upgrade cilium cilium/cilium -n kube-system --reuse-values --set key=value

# Rollback
helm rollback cilium -n kube-system
```

---

**Congratulations!** You've successfully deployed Cilium with OCI IPAM. 🎉



---

**Version**: 1.0  
**Last Updated**: October 27, 2025
