# Cilium OCI IPAM Troubleshooting Guide

Comprehensive troubleshooting guide for Cilium OCI IPAM issues.

**Version**: Cilium v1.15.2  
**Last Updated**: October 27, 2025

---

## Table of Contents

- [Diagnostic Tools](#diagnostic-tools)
- [Common Issues](#common-issues)
- [IAM and Permissions](#iam-and-permissions)
- [VNIC Issues](#vnic-issues)
- [IP Allocation Issues](#ip-allocation-issues)
- [Subnet Tags Issues](#subnet-tags-issues)
- [Network Connectivity](#network-connectivity)
- [Performance Issues](#performance-issues)

---

## Diagnostic Tools

### Essential Commands

```bash
# 1. Check Cilium status
cilium status

# 2. List CiliumNodes
kubectl get ciliumnode

# 3. View specific node details
kubectl get ciliumnode <node-name> -o yaml

# 4. Check VNIC information
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq

# 5. Operator logs
kubectl logs -n kube-system deployment/cilium-operator --tail=100

# 6. Agent logs (specific node)
kubectl logs -n kube-system daemonset/cilium -c cilium-agent \
  -l kubernetes.io/hostname=<node-name> --tail=100

# 7. Check IAM permissions
oci iam region list --auth instance_principal

# 8. Pod events
kubectl describe pod <pod-name>
```

### Diagnostic Script

```bash
#!/bin/bash
# save as: diagnose-cilium-oci.sh

echo "=== Cilium Status ==="
cilium status

echo ""
echo "=== CiliumNodes ==="
kubectl get ciliumnode -o wide

echo ""
echo "=== Operator Logs (last 50 lines) ==="
kubectl logs -n kube-system deployment/cilium-operator --tail=50

echo ""
echo "=== Agent Pods ==="
kubectl get pods -n kube-system -l k8s-app=cilium

echo ""
echo "=== Failed Pods (if any) ==="
kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded

echo ""
echo "=== Recent Events ==="
kubectl get events -A --sort-by='.lastTimestamp' | tail -20
```

Run: `chmod +x diagnose-cilium-oci.sh && ./diagnose-cilium-oci.sh`

---

## Common Issues

### Issue 1: Pods Stuck in ContainerCreating

#### Symptoms

```
NAME                     READY   STATUS              RESTARTS   AGE
test-pod-12345-abcde     0/1     ContainerCreating   0          2m
```

#### Diagnosis

```bash
# 1. Check pod events
kubectl describe pod <pod-name>

# Look for errors like:
# "Failed to create pod sandbox: IP allocation failed"
# "Waiting for IP address"

# 2. Check CiliumNode status
kubectl get ciliumnode <node-name> -o yaml

# 3. Check Operator logs
kubectl logs -n kube-system deployment/cilium-operator | grep -i "error\|failed"
```

#### Possible Causes and Solutions

**Cause 1: IP Address Exhaustion**

```bash
# Check IP usage
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq

# Look for: allocated-ips >= available-ips
```

**Solution**:
- Expand subnet CIDR (use /24 instead of /28)
- Add more subnets with Subnet Tags
- Delete unused Pods to free IPs

**Cause 2: IAM Permission Issues**

```bash
# Test permissions
oci iam region list --auth instance_principal

# Should return region list, not "Unauthorized"
```

**Solution**:
- Check Dynamic Group includes your instances
- Verify Policy statements (see [IAM section](#iam-and-permissions))

**Cause 3: VNIC Limit Reached**

```bash
# Check VNIC count
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq 'length'

# Compare with instance shape limit
```

**Solution**:
- Upgrade to instance shape with more VNICs
- Use larger subnets to reduce VNIC need

---

### Issue 2: Operator Crashes or Restarts

#### Symptoms

```bash
kubectl get pods -n kube-system -l name=cilium-operator

# Shows frequent restarts
```

#### Diagnosis

```bash
# Check Operator logs
kubectl logs -n kube-system deployment/cilium-operator --previous

# Check resource usage
kubectl top pod -n kube-system -l name=cilium-operator
```

#### Possible Causes

**Cause 1: Out of Memory**

**Solution**: Increase Operator memory:

```yaml
operator:
  resources:
    requests:
      memory: 512Mi
    limits:
      memory: 1Gi
```

**Cause 2: OCI API Rate Limiting**

Look for: "Rate limit exceeded" in logs

**Solution**: Increase `ipAllocationTimeout` and add backoff:

```yaml
oci:
  ipAllocationTimeout: 120
```

---

### Issue 3: Agent Pod CrashLoopBackOff

#### Symptoms

```bash
kubectl get pods -n kube-system -l k8s-app=cilium

# Shows CrashLoopBackOff
```

#### Diagnosis

```bash
# Check logs
kubectl logs -n kube-system daemonset/cilium -c cilium-agent --tail=100

# Check if it's a specific node
kubectl get pods -n kube-system -l k8s-app=cilium -o wide
```

#### Solutions

**Issue**: BPF filesystem not mounted

**Solution**: Ensure BPF is mounted on nodes:

```bash
# On the problematic node
mount | grep /sys/fs/bpf

# If not mounted
mount bpffs -t bpf /sys/fs/bpf
```

---

## IAM and Permissions

### Issue: "Unauthorized" or "Forbidden" Errors

#### Symptoms

Operator logs show:
```
level=error msg="Failed to list subnets" error="Unauthorized"
```

#### Diagnosis

```bash
# Test Instance Principal
oci iam region list --auth instance_principal

# If this fails, IAM is not configured correctly
```

#### Solutions

**Step 1: Verify Dynamic Group**

OCI Console → Identity → Dynamic Groups

Check matching rules include your instances:

```
instance.compartment.id = 'ocid1.compartment.oc1..aaaaaaaa...'
```

Or for specific instances:

```
matching_instance_id = 'ocid1.instance.oc1.region.anzxsljr...'
```

**Step 2: Verify Policy**

OCI Console → Identity → Policies

Ensure these statements exist:

```
Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use subnets in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use private-ips in compartment <compartment-name>
```

**Step 3: Check Policy Scope**

- Policy must be in the **same compartment** or parent compartment as VCN
- Use `manage vnics` not just `use vnics`

**Step 4: Wait for Propagation**

IAM changes take 1-2 minutes to propagate. Wait and retry.

### Required IAM Permissions

| Permission | Purpose |
|------------|---------|
| `manage vnics` | Create, attach, detach VNICs |
| `use subnets` | Query subnet information |
| `use network-security-groups` | Apply NSGs to VNICs |
| `use private-ips` | Allocate secondary IPs |

---

## VNIC Issues

### Issue: VNICs Not Created Automatically

#### Symptoms

- IP usage is high but no new VNICs created
- Operator logs show subnet selection but no VNIC creation

#### Diagnosis

```bash
# Check current VNICs
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq

# Check Operator logs
kubectl logs -n kube-system deployment/cilium-operator | grep -i "vnic\|subnet"
```

#### Possible Causes

**Cause 1: Subnet Tags Not Configured**

```bash
# Check Operator args
kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter

# If you see: --subnet-tags-filter=''
# Then Subnet Tags are not configured correctly
```

**Solution**: See [Subnet Tags Issues](#subnet-tags-issues)

**Cause 2: Instance VNIC Limit Reached**

```bash
# Count current VNICs
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq 'length'
```

**Solution**: Upgrade instance shape or delete unused VNICs

**Cause 3: No Available Subnets**

**Solution**: 
- Create more subnets in your VCN
- Tag subnets with correct tags
- Ensure subnets have available IPs

---

### Issue: Multiple VNICs Created Unexpectedly

#### Symptoms

Expected 1 additional VNIC, but 2+ were created

#### Root Cause

This happens when:
1. Using small subnets (/28 = 13 IPs)
2. Cilium's surge allocation tries to allocate 14+ IPs
3. First subnet exhausted → creates VNIC1
4. Second subnet exhausted → creates VNIC2

#### Solution

✅ **Use /24 or larger subnets** (250+ IPs)

```bash
# Check your subnet sizes
oci network subnet get --subnet-id <subnet-ocid> --query 'data."cidr-block"'
```

Recommended subnet sizes:

| Size | Usable IPs | Recommended For |
|------|-----------|-----------------|
| /28 | 13 | ❌ Too small |
| /27 | 29 | ⚠️ Development only |
| /26 | 61 | ⚠️ Small clusters |
| /24 | 251 | ✅ Production (recommended) |
| /23 | 509 | ✅ Large clusters |

---

### Issue: VNIC Creation Stuck

#### Symptoms

Operator logs show "Creating VNIC..." but never completes

#### Diagnosis

```bash
# Check Operator logs
kubectl logs -n kube-system deployment/cilium-operator | grep "Creating VNIC"

# Check OCI Console for VNIC attachment status
```

#### Solutions

**Timeout Issue**: Increase timeout:

```yaml
oci:
  ipAllocationTimeout: 120  # 2 minutes
```

**OCI API Issue**: Check OCI service health dashboard

---

## IP Allocation Issues

### Issue: "No More IPs Available"

#### Symptoms

```
level=error msg="Unable to assign additional IPs to interface"
error="All IPs have already been allocated"
```

#### Diagnosis

```bash
# Check subnet IP usage
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | \
  jq '.[] | {subnet: .subnet.cidr, allocated: ."allocated-ips", available: ."available-ips"}'
```

#### Solutions

**Solution 1: Expand Subnet**

Can't expand existing subnet, must create new one:

```bash
# Create larger subnet
oci network subnet create \
  --compartment-id <compartment-ocid> \
  --vcn-id <vcn-ocid> \
  --cidr-block "10.0.10.0/24" \
  --display-name "cilium-pod-subnet-large" \
  --freeform-tags '{"cilium-pod-network":"yes"}' \
  --auth instance_principal
```

**Solution 2: Delete Unused Pods**

```bash
# Find completed/failed pods
kubectl get pods -A --field-selector=status.phase=Succeeded
kubectl get pods -A --field-selector=status.phase=Failed

# Delete them to free IPs
kubectl delete pod <pod-name> -n <namespace>
```

---

### Issue: IP Allocation Slow

#### Symptoms

Pods take >5 seconds to get IP addresses

#### Diagnosis

```bash
# Check allocation latency in Operator logs
kubectl logs -n kube-system deployment/cilium-operator | grep "IP allocation"
```

#### Solutions

**Enable VNIC Pre-allocation**:

```yaml
oci:
  vnicPreAllocationThreshold: 0.7  # Pre-create at 70%
```

**Increase Operator Resources**:

```yaml
operator:
  resources:
    requests:
      cpu: 1000m
      memory: 1Gi
```

---

## Subnet Tags Issues

### Issue: Subnet Tags Not Working

#### Symptoms

- Configured `oci.subnetTags` but VNICs not created from tagged subnets
- Operator logs show empty `--subnet-tags-filter`

#### Diagnosis

```bash
# Check Operator args
kubectl logs -n kube-system deployment/cilium-operator | head -20 | grep subnet-tags-filter

# ❌ If you see: --subnet-tags-filter=''
# ✅ Should see: --subnet-tags-filter='your-tag=value'
```

#### Root Cause

**Cilium's architecture**: Agent and Operator are separate components.

- `oci.subnetTags` configures the Agent (ConfigMap)
- Operator reads its own command-line args (`operator.extraArgs`)
- Helm Chart doesn't auto-propagate `oci.subnetTags` to Operator

#### Solution

**Must configure BOTH places**:

```yaml
oci:
  subnetTags:
    cilium-pod-network: "yes"  # Config 1: For Agent

operator:
  extraArgs:
    - --subnet-tags-filter=cilium-pod-network=yes  # Config 2: For Operator ⚠️
```

Verify after applying:

```bash
kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter
# Should see: --subnet-tags-filter='cilium-pod-network=yes'
```

---

### Issue: Subnets Not Matched by Tags

#### Diagnosis

```bash
# Check subnet tags
oci network subnet get \
  --subnet-id <subnet-ocid> \
  --query 'data."freeform-tags"' \
  --auth instance_principal

# Should show your tags
```

#### Solutions

**Add tags to subnets**:

```bash
oci network subnet update \
  --subnet-id <subnet-ocid> \
  --freeform-tags '{"cilium-pod-network":"yes"}' \
  --force \
  --auth instance_principal
```

**Verify tag format**:
- Key: `cilium-pod-network`
- Value: `"yes"` (string)
- Match exactly in Operator args: `--subnet-tags-filter=cilium-pod-network=yes`

---

## Network Connectivity

### Issue: Pod-to-Pod Communication Fails

#### Symptoms

```bash
kubectl exec pod1 -- ping <pod2-ip>
# Timeout or unreachable
```

#### Diagnosis

```bash
# 1. Check if both Pods have IPs
kubectl get pods -o wide

# 2. Check routing
kubectl exec pod1 -- ip route

# 3. Check Cilium status
cilium status

# 4. Use Hubble to observe flows
hubble observe --pod pod1
```

#### Solutions

**Check Security Lists/NSGs**:
- OCI Security Lists must allow traffic between Pod subnets
- Network Security Groups (if used) must allow Pod-to-Pod traffic

**Check Cilium Network Policies**:
```bash
kubectl get networkpolicy -A
```

---

### Issue: Pod Cannot Reach Internet

#### Diagnosis

```bash
kubectl exec <pod> -- ping -c 3 8.8.8.8
```

#### Solutions

**Check NAT Gateway**:
- Subnet route table must have route to NAT Gateway or Internet Gateway
- NAT Gateway must be in AVAILABLE state

**Check Masquerading**:

Ensure masquerading is enabled:

```yaml
enableIPv4Masquerade: true
```

---

## Performance Issues

### Issue: High Operator CPU Usage

#### Diagnosis

```bash
kubectl top pod -n kube-system -l name=cilium-operator
```

#### Solutions

**Increase resources**:

```yaml
operator:
  resources:
    requests:
      cpu: 1000m
      memory: 1Gi
    limits:
      cpu: 2000m
      memory: 2Gi
```

**Reduce log verbosity**:

Remove `--debug` from `operator.extraArgs` if present.

---

### Issue: Slow Pod Creation

#### Symptoms

Pods take >30 seconds to start

#### Diagnosis

```bash
# Time Pod creation
time kubectl run test --image=nginx

# Check Operator response time
kubectl logs -n kube-system deployment/cilium-operator | grep "IP allocation"
```

#### Solutions

**Pre-allocate VNICs**:

```yaml
oci:
  vnicPreAllocationThreshold: 0.7
```

**Increase API timeout**:

```yaml
oci:
  ipAllocationTimeout: 120
```

---

## Getting Help

### Information to Collect

When requesting support, collect:

```bash
# 1. Cilium version
helm list -n kube-system

# 2. Configuration
helm get values cilium -n kube-system > cilium-values.yaml

# 3. CiliumNode status
kubectl get ciliumnode -o yaml > ciliumnodes.yaml

# 4. Operator logs
kubectl logs -n kube-system deployment/cilium-operator --tail=500 > operator.log

# 5. Agent logs (problematic node)
kubectl logs -n kube-system daemonset/cilium -c cilium-agent \
  -l kubernetes.io/hostname=<node> --tail=500 > agent.log

# 6. Recent events
kubectl get events -A --sort-by='.lastTimestamp' > events.txt

# 7. Failing Pod describe
kubectl describe pod <failing-pod> > pod-describe.txt
```

### Contact

 
- **GitHub Issues**: https://github.com/iafboy/oci_cilium/issues
 

---

## Quick Reference

### Diagnostic Flowchart

```
Pod stuck in ContainerCreating?
  │
  ├─→ Check kubectl describe pod
  │     │
  │     ├─→ "IP allocation failed"
  │     │     └─→ Check CiliumNode VNIC IPs
  │     │           ├─→ All IPs used? → Expand subnets
  │     │           └─→ No VNICs? → Check IAM permissions
  │     │
  │     └─→ "Waiting for sandbox"
  │           └─→ Check Agent logs
  │
  └─→ Check Operator logs
        ├─→ "Unauthorized" → IAM issue
        ├─→ "subnet-tags-filter=''" → Missing operator.extraArgs
        └─→ "VNIC limit reached" → Upgrade instance shape
```

### Common Log Messages

| Log Message | Meaning | Action |
|-------------|---------|--------|
| `All X non-reserved IP addresses have been allocated` | Subnet full | Expand subnet or add new one |
| `Unauthorized` / `Forbidden` | IAM issue | Check Dynamic Group and Policy |
| `Unable to assign additional IPs to interface` | VNIC full or subnet full | Create new VNIC or expand subnet |
| `VNIC limit reached` | Instance shape limit | Upgrade instance shape |
| `subnet-tags-filter=''` | Subnet Tags misconfigured | Add `operator.extraArgs` |

---

**Version**: 1.0  
**Last Updated**: October 27, 2025  
**Maintainer**: Dengwei (SEHUB)
