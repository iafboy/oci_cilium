# OCI IPAM Troubleshooting Guide

This guide helps you diagnose and resolve common issues with Cilium OCI IPAM.

## Table of Contents

- [Initial Setup Issues](#initial-setup-issues)
- [Authentication and Authorization](#authentication-and-authorization)
- [VNIC and IP Allocation](#vnic-and-ip-allocation)
- [Network Connectivity](#network-connectivity)
- [Performance Issues](#performance-issues)
- [Debugging Tools](#debugging-tools)

---

## Initial Setup Issues

### Issue: Operator fails to start with "OCI VCN ID is required"

**Symptoms:**
```
level=error msg="OCI VCN ID is required but not configured. Please set --oci-vcn-id operator flag"
```

**Cause:** The `--oci-vcn-id` flag is not set in operator configuration.

**Solution:**

1. **Get your VCN OCID from OCI Console:**
   ```bash
   oci network vcn list --compartment-id <your-compartment-id>
   ```

2. **Update Helm values:**
   ```yaml
   operator:
     extraArgs:
       - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...
   ```

3. **Upgrade Cilium:**
   ```bash
   helm upgrade cilium cilium/cilium -n kube-system -f values.yaml
   ```

---

### Issue: "failed to search VCN resources" error

**Symptoms:**
```
level=error msg="failed to search VCN resources: Service error:NotAuthenticated"
```

**Cause:** Operator cannot authenticate to OCI API.

**Solutions:**

**If using Instance Principals:**

1. **Verify instance is in dynamic group:**
   ```bash
   # Check instance OCID
   oci compute instance list --compartment-id <compartment-id>
   
   # Verify dynamic group includes this instance
   oci iam dynamic-group get --dynamic-group-id <group-id>
   ```

2. **Check IAM policies:**
   ```hcl
   Allow dynamic-group cilium-operator-group to read virtual-network-family in compartment <name>
   Allow dynamic-group cilium-operator-group to manage vnics in compartment <name>
   ```

**If using config file:**

1. **Set authentication mode:**
   ```yaml
   OCIUseInstancePrincipal: false
   ```

2. **Verify config file exists on operator pod:**
   ```bash
   kubectl exec -n kube-system deployment/cilium-operator -- cat ~/.oci/config
   ```

---

### Issue: "empty shape list returned by ListShapes"

**Symptoms:**
```
level=error msg="empty shape list returned by ListShapes"
```

**Cause:** Operator cannot query compute shapes, usually due to missing permissions.

**Solution:**

1. **Add compute permissions to IAM policy:**
   ```hcl
   Allow dynamic-group cilium-operator-group to read compute-management-family in compartment <name>
   ```

2. **Restart operator:**
   ```bash
   kubectl rollout restart -n kube-system deployment/cilium-operator
   ```

---

## Authentication and Authorization

### Issue: "failed to create instance principal config provider"

**Symptoms:**
```
level=error msg="failed to create instance principal config provider: failed to get instance metadata"
```

**Diagnostic Steps:**

1. **Check metadata service accessibility:**
   ```bash
   # From a node
   curl -H "Authorization: Bearer Oracle" http://169.254.169.254/opc/v2/instance/
   ```

2. **Verify instance metadata:**
   ```bash
   kubectl exec -n kube-system <cilium-operator-pod> -- \
     curl -H "Authorization: Bearer Oracle" \
     http://169.254.169.254/opc/v2/instance/id
   ```

**Solutions:**

1. **Ensure operator pod has host network access** (not required by default, but helps debug):
   ```yaml
   operator:
     hostNetwork: false  # Usually false is correct
   ```

2. **Check dynamic group matching rule:**
   ```hcl
   # Match all instances in compartment
   ALL {instance.compartment.id = 'ocid1.compartment.oc1..xxx'}
   
   # OR match by tag
   ALL {tag.namespace.key = 'value'}
   ```

3. **Fallback to config file auth:**
   ```yaml
   OCIUseInstancePrincipal: false
   ```

---

### Issue: "Service error:NotAuthorizedOrNotFound"

**Symptoms:**
```
level=error msg="Service error:NotAuthorizedOrNotFound. The requested resource is not found or you are not authorized to access it"
```

**Cause:** Missing IAM permissions or resource doesn't exist in specified compartment.

**Solution:**

1. **Verify all required policies exist:**
   ```hcl
   # Virtual Network permissions
   Allow dynamic-group cilium-operator-group to manage vnics in compartment <name>
   Allow dynamic-group cilium-operator-group to use subnets in compartment <name>
   Allow dynamic-group cilium-operator-group to use private-ips in compartment <name>
   Allow dynamic-group cilium-operator-group to read virtual-network-family in compartment <name>
   
   # Compute permissions
   Allow dynamic-group cilium-operator-group to read compute-management-family in compartment <name>
   Allow dynamic-group cilium-operator-group to read instances in compartment <name>
   
   # Search permissions
   Allow dynamic-group cilium-operator-group to read search-resources in tenancy
   ```

2. **Verify VCN exists in the correct compartment:**
   ```bash
   oci network vcn get --vcn-id <your-vcn-id>
   ```

3. **Check compartment hierarchy:**
   - Policies may need to be at parent compartment level
   - Use `in tenancy` if resources span multiple compartments

---

## VNIC and IP Allocation

### Issue: "unable to find matching subnet"

**Symptoms:**
```
level=error msg="unable to find matching subnet available for interface creation"
```

**Diagnostic Steps:**

1. **List available subnets:**
   ```bash
   oci network subnet list \
     --compartment-id <compartment-id> \
     --vcn-id <vcn-id>
   ```

2. **Check subnet available IPs:**
   ```bash
   oci network subnet get --subnet-id <subnet-id> \
     --query 'data.{"CIDR":"cidr-block","Available":"available-ip-address-count"}'
   ```

**Common Causes:**

1. **All subnets are full:**
   - Each subnet reserves 3 IPs for OCI
   - Available IPs = Total IPs - Used IPs - 3
   
   **Solution:** Create additional subnets or expand existing ones

2. **Availability domain mismatch:**
   - Node is in AD-1 but subnets are in AD-2
   
   **Solution:** Use regional subnets or create AD-specific subnets

3. **Subnet tags don't match:**
   ```yaml
   # CiliumNode expects these tags
   spec:
     oci:
       subnet-tags:
         environment: production  # Must match subnet tags
   ```
   
   **Solution:** Remove subnet-tags requirement or add tags to subnets

---

### Issue: Pods stuck in "ContainerCreating" state

**Symptoms:**
```bash
kubectl get pods
NAME          READY   STATUS              RESTARTS   AGE
nginx-xxx     0/1     ContainerCreating   0          2m
```

**Diagnostic Steps:**

1. **Check pod events:**
   ```bash
   kubectl describe pod <pod-name>
   # Look for: "failed to allocate IP"
   ```

2. **Check CiliumNode status:**
   ```bash
   kubectl get ciliumnode <node-name> -o jsonpath='{.status.ipam}' | jq
   ```

3. **Check operator logs:**
   ```bash
   kubectl logs -n kube-system deployment/cilium-operator | grep -i "error\|fail"
   ```

**Solutions:**

1. **VNIC limit reached:**
   ```bash
   # Check current VNIC count
   kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq 'length'
   ```
   
   **Solution:** 
   - Drain node and move pods elsewhere
   - Use larger instance shape with more VNIC capacity

2. **IP allocation timeout:**
   ```bash
   # Check recent allocations
   kubectl logs -n kube-system deployment/cilium-operator | grep "AssignPrivateIPAddresses"
   ```
   
   **Solution:** Increase operator timeout settings

3. **Subnet exhaustion:**
   **Solution:** Add more subnets to VCN

---

### Issue: "unable to attach VNIC" or "Wait for VNIC attach failed"

**Symptoms:**
```
level=error msg="unable to attach VNIC: Service error:LimitExceeded"
```

**Causes:**

1. **Instance shape VNIC limit reached**
2. **Subnet service limits exceeded**
3. **Concurrent attachment conflicts**

**Solutions:**

1. **Check instance shape limits:**
   ```bash
   # Get instance shape
   kubectl get ciliumnode <node-name> -o jsonpath='{.spec.oci.instance-type}'
   
   # Check shape limits
   oci compute shape list --compartment-id <compartment-id> \
     --query "data[?shape=='VM.Standard.E4.Flex'].{Shape:shape,VNICs:\"max-vnic-attachments\"}"
   ```

2. **Check service limits:**
   ```bash
   oci limits value list \
     --compartment-id <compartment-id> \
     --service-name compute
   ```

3. **Request limit increase** via OCI Console

4. **Use larger shapes:**
   ```yaml
   # For node pools or instance configurations
   shape: VM.Standard.E4.Flex
   shape-config:
     ocpus: 8  # More OCPUs = more VNICs
   ```

---

### Issue: High VNIC creation rate (cost concern)

**Symptoms:**
- Many VNICs with few IPs each
- Unnecessary VNIC churn

**Solutions:**

1. **Increase IP pre-allocation:**
   ```yaml
   operator:
     extraArgs:
       - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
       - --nodes-ipam-pod-cidr-allocation-threshold=10  # Pre-allocate 10 IPs
   ```

2. **Adjust release threshold:**
   ```yaml
   operator:
     extraArgs:
       - --nodes-ipam-pod-cidr-release-threshold=5  # Keep 5 free IPs
   ```

3. **Monitor VNIC usage:**
   ```bash
   kubectl get ciliumnodes -o json | \
     jq '.items[] | {name: .metadata.name, vnics: (.status.oci.vnics | length)}'
   ```

---

## Network Connectivity

### Issue: Pods cannot reach other pods

**Diagnostic Steps:**

1. **Check pod IPs:**
   ```bash
   kubectl get pods -o wide
   ```

2. **Verify IPs are from VCN subnets:**
   ```bash
   # Pod IPs should be in VCN CIDR range
   kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq
   ```

3. **Test connectivity:**
   ```bash
   kubectl exec -it <pod1> -- ping <pod2-ip>
   ```

**Solutions:**

1. **Check security lists on subnets:**
   - Allow ingress from VCN CIDR
   - Allow egress to VCN CIDR

2. **Verify route tables:**
   - Ensure proper routing for pod subnets
   - Check for conflicting routes

3. **Check Cilium policy:**
   ```bash
   kubectl get networkpolicies --all-namespaces
   ```

---

### Issue: Pods cannot reach external services

**Diagnostic Steps:**

1. **Test DNS resolution:**
   ```bash
   kubectl exec -it <pod> -- nslookup google.com
   ```

2. **Test external connectivity:**
   ```bash
   kubectl exec -it <pod> -- curl -I https://google.com
   ```

3. **Check NAT gateway configuration:**
   ```bash
   oci network nat-gateway list --compartment-id <compartment-id>
   ```

**Solutions:**

1. **Add NAT gateway to VCN:**
   - For pods in private subnets to reach internet
   - Configure route table: 0.0.0.0/0 → NAT Gateway

2. **Check egress rules:**
   - Security lists must allow egress to 0.0.0.0/0
   - Or specific destination CIDRs

3. **Verify DNS:**
   ```yaml
   # In Cilium config
   dnsPolicy: ClusterFirst  # Or ClusterFirstWithHostNet
   ```

---

### Issue: External services cannot reach pods

**Symptoms:**
- Load balancer health checks fail
- Direct pod access from VCN doesn't work

**Solutions:**

1. **Verify security lists allow inbound traffic:**
   ```hcl
   # Security List rule
   Source: 0.0.0.0/0 (or specific CIDR)
   Protocol: TCP
   Destination Port: <service-port>
   ```

2. **Check service type:**
   ```bash
   kubectl get svc
   # Type should be LoadBalancer for external access
   ```

3. **Verify pod IP is reachable:**
   ```bash
   # From another OCI instance in same VCN
   ping <pod-ip>
   ```

---

## Performance Issues

### Issue: Slow pod startup times

**Symptoms:**
- Pods take >30 seconds to get IP addresses
- Frequent VNIC creation

**Diagnostic Steps:**

1. **Check VNIC allocation time:**
   ```bash
   kubectl logs -n kube-system deployment/cilium-operator | \
     grep "Attached VNIC" | tail -20
   ```

2. **Monitor IPAM metrics:**
   ```bash
   kubectl port-forward -n kube-system deployment/cilium-operator 9963:9963
   curl http://localhost:9963/metrics | grep ipam
   ```

**Solutions:**

1. **Pre-allocate IPs:**
   ```yaml
   operator:
     extraArgs:
       - --nodes-ipam-pod-cidr-allocation-threshold=15
       - --parallel-alloc-workers=10
   ```

2. **Use multiple subnets:**
   - Distribute load across subnets
   - Reduces contention

3. **Right-size instance shapes:**
   - Use shapes with more VNIC capacity
   - Fewer VNIC creations needed

---

### Issue: Operator consuming high CPU/memory

**Symptoms:**
```bash
kubectl top pods -n kube-system | grep cilium-operator
# Shows high CPU or memory usage
```

**Solutions:**

1. **Adjust resource limits:**
   ```yaml
   operator:
     resources:
       limits:
         cpu: 2000m
         memory: 2Gi
       requests:
         cpu: 200m
         memory: 256Mi
   ```

2. **Reduce reconciliation frequency:**
   ```yaml
   operator:
     extraArgs:
       - --nodes-ipam-sync-interval=60s  # Default: 30s
   ```

3. **Check for error loops:**
   ```bash
   kubectl logs -n kube-system deployment/cilium-operator | \
     grep -i error | sort | uniq -c | sort -rn
   ```

---

## Debugging Tools

### Collect Operator Logs

```bash
# Last 1000 lines
kubectl logs -n kube-system deployment/cilium-operator --tail=1000

# With timestamps
kubectl logs -n kube-system deployment/cilium-operator --timestamps=true

# Follow logs
kubectl logs -n kube-system deployment/cilium-operator -f

# Previous pod (if crashed)
kubectl logs -n kube-system deployment/cilium-operator --previous
```

### Inspect CiliumNode Resources

```bash
# List all nodes
kubectl get ciliumnodes

# Full YAML of a node
kubectl get ciliumnode <node-name> -o yaml

# Just OCI status
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci}' | jq

# IPAM status
kubectl get ciliumnode <node-name> -o jsonpath='{.status.ipam}' | jq

# Check all nodes' VNIC counts
kubectl get ciliumnodes -o json | \
  jq -r '.items[] | "\(.metadata.name): \(.status.oci.vnics | length) VNICs"'
```

### Check OCI Resources

```bash
# List VNICs for an instance
oci compute vnic-attachment list \
  --compartment-id <compartment-id> \
  --instance-id <instance-id>

# Get VNIC details
oci network vnic get --vnic-id <vnic-id>

# List private IPs on a VNIC
oci network private-ip list --vnic-id <vnic-id>

# Check subnet capacity
oci network subnet get --subnet-id <subnet-id> | \
  jq '.data | {cidr: ."cidr-block", available: ."available-ip-address-count"}'
```

### Enable Debug Logging

```yaml
# In Helm values
debug:
  enabled: true
  
operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
    - --debug=true
```

### Cilium CLI Debugging

```bash
# Install Cilium CLI
curl -L --remote-name-all https://github.com/cilium/cilium-cli/releases/latest/download/cilium-linux-amd64.tar.gz
tar xzvf cilium-linux-amd64.tar.gz
sudo mv cilium /usr/local/bin/

# Check status
cilium status

# Connectivity test
cilium connectivity test

# Debug information
cilium sysdump
```

### Network Troubleshooting

```bash
# From a pod - test connectivity
kubectl run -it --rm debug --image=nicolaka/netshoot --restart=Never -- /bin/bash

# Inside the debug pod:
ping <pod-ip>
traceroute <pod-ip>
nslookup <service-name>
curl <service-url>
ip addr
ip route
```

### Verify OCI API Access

```bash
# Execute in operator pod
kubectl exec -n kube-system deployment/cilium-operator -it -- /bin/bash

# Inside pod:
# Check metadata access
curl -H "Authorization: Bearer Oracle" \
  http://169.254.169.254/opc/v2/instance/

# Test instance principal (if using)
export OCI_CLI_AUTH=instance_principal
oci iam region list
```

---

## Common Error Messages Reference

| Error Message | Likely Cause | Quick Fix |
|---------------|--------------|-----------|
| `OCI VCN ID is required` | Missing --oci-vcn-id flag | Add flag to operator args |
| `NotAuthenticated` | IAM auth failure | Check instance principals/policies |
| `NotAuthorizedOrNotFound` | Missing permissions | Add required IAM policies |
| `LimitExceeded` | VNIC or IP limit reached | Use larger shape or request limit increase |
| `unable to find matching subnet` | No suitable subnet | Check subnet capacity and tags |
| `failed to create instance principal` | Not in dynamic group | Add instance to dynamic group |
| `empty shape list` | Missing compute permissions | Add compute read permissions |
| `WaitVNICAttached timeout` | VNIC attachment slow | Check OCI service status |

---

## Getting More Help

1. **Increase Log Verbosity:**
   ```yaml
   debug:
     enabled: true
     verbose: datapath  # Or: flow, kvstore, envoy, etc.
   ```

2. **Collect Support Bundle:**
   ```bash
   cilium sysdump
   # Creates a zip file with all debug info
   ```

3. **Check OCI Service Health:**
   - Visit: https://ocistatus.oraclecloud.com/

4. **Community Support:**
   - Cilium Slack: https://cilium.io/slack
   - GitHub Issues: https://github.com/cilium/cilium/issues

5. **Report Issue Template:**
   ```
   Environment:
   - Cilium version: 1.15.2
   - K8s version: 
   - OCI region: 
   - Instance shape: 
   
   Configuration:
   - VCN OCID: 
   - Auth method: Instance Principal / Config File
   
   Issue Description:
   [Describe the problem]
   
   Steps to Reproduce:
   1. 
   2. 
   3. 
   
   Logs:
   [Paste relevant logs]
   
   Expected Behavior:
   [What should happen]
   
   Actual Behavior:
   [What actually happens]
   ```

---

## Prevention Best Practices

1. **Monitor VNIC usage** - Set up alerts before limits are reached
2. **Use regional subnets** - More flexible than AD-specific
3. **Right-size instance shapes** - Plan for peak pod count
4. **Regular subnet audits** - Ensure adequate IP space
5. **Test IAM policies** - Verify before production deployment
6. **Document custom configurations** - For troubleshooting later
7. **Enable metrics** - Track IPAM performance over time
8. **Implement GitOps** - Version control all configurations

---

## Related Documentation

- [Quick Start Guide](quickstart.md)
- [Configuration Reference](configuration.md)
- [OCI IPAM Overview](README.md)
