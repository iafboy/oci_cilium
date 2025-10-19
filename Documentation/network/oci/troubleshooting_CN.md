# OCI IPAM 故障排查指南

诊断和解决 Cilium OCI IPAM 常见问题。

## 目录

- [诊断工具](#诊断工具)
- [常见错误](#常见错误)
- [IPAM 问题](#ipam-问题)
- [网络问题](#网络问题)
- [性能问题](#性能问题)
- [配置问题](#配置问题)
- [高级调试](#高级调试)

## 诊断工具

### 基本检查

```bash
# 1. 检查 Cilium Pod 状态
kubectl -n kube-system get pods -l k8s-app=cilium

# 2. 检查 Cilium Operator 状态
kubectl -n kube-system get pods -l name=cilium-operator

# 3. 查看 Cilium 状态
kubectl -n kube-system exec -it ds/cilium -- cilium status

# 4. 检查 IPAM 模式
kubectl -n kube-system get cm cilium-config -o yaml | grep ipam

# 5. 查看 CiliumNode 资源
kubectl get ciliumnodes -o wide
```

### 日志收集

```bash
# Cilium Operator 日志（IPAM 分配）
kubectl -n kube-system logs deployment/cilium-operator --tail=100 -f

# Cilium Agent 日志（网络配置）
kubectl -n kube-system logs ds/cilium --tail=100 -f

# 过滤 OCI 相关日志
kubectl -n kube-system logs deployment/cilium-operator | grep -i oci

# 导出完整日志用于分析
kubectl -n kube-system logs deployment/cilium-operator > operator.log
kubectl -n kube-system logs ds/cilium --all-containers > agent.log
```

### CiliumNode 检查

```bash
# 获取所有节点的详细 OCI 状态
kubectl get ciliumnodes -o yaml > ciliumnodes.yaml

# 检查特定节点的 VNIC
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq

# 检查 VNIC 限制
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnic-limits}' | jq

# 检查 IP 分配
kubectl get ciliumnode <node-name> -o jsonpath='{.status.ipam}' | jq
```

### OCI CLI 验证

```bash
# 列出实例 VNIC
oci compute vnic-attachment list \
  --instance-id <instance-ocid> \
  --compartment-id <compartment-ocid>

# 获取 VNIC 详细信息
oci network vnic get --vnic-id <vnic-ocid>

# 检查子网可用 IP
oci network subnet get --subnet-id <subnet-ocid> \
  --query 'data."available-ips"'

# 验证 VCN 配置
oci network vcn get --vcn-id <vcn-ocid>
```

## 常见错误

### 错误 1: "failed to allocate IP: no available subnets"

**症状**: Pod 因无法分配 IP 而卡在 ContainerCreating

**原因**: VCN 中所有子网的 IP 都已用完

**诊断**:
```bash
# 检查子网可用 IP
oci network subnet list --vcn-id <vcn-ocid> --compartment-id <compartment-ocid> \
  --query 'data[*].{Name:"display-name", CIDR:"cidr-block", AvailableIPs:"available-ips"}' \
  --output table
```

**解决方案**:
1. **选项 A**: 向 VCN 添加新子网
   ```bash
   oci network subnet create \
     --vcn-id <vcn-ocid> \
     --cidr-block "10.0.X.0/24" \
     --compartment-id <compartment-ocid> \
     --display-name "cilium-pod-subnet-X"
   ```

2. **选项 B**: 释放未使用的 IP
   ```bash
   # 查找未附加的私有 IP
   oci network private-ip list \
     --subnet-id <subnet-ocid> \
     --query 'data[?!"vnic-id"].id' \
     --raw-output
   
   # 删除未使用的 IP
   oci network private-ip delete --private-ip-id <ip-ocid>
   ```

3. **选项 C**: 扩展子网 CIDR（需要重新创建）

---

### 错误 2: "VNIC attachment failed: LimitExceeded"

**症状**: 无法创建新 VNIC，节点达到最大 VNIC 数

**原因**: OCI 实例形状达到了最大 VNIC 限制

**诊断**:
```bash
# 检查当前 VNIC 数
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq 'length'

# 检查 VNIC 限制
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnic-limits.max-vnics}'

# 使用 OCI CLI
oci compute vnic-attachment list \
  --instance-id <instance-ocid> \
  --compartment-id <compartment-ocid> | jq 'length'
```

**解决方案**:
1. **选项 A**: 使用更大的实例形状
   ```bash
   # 检查形状限制
   # VM.Standard.E4.Flex (2 OCPU): 2 VNICs
   # VM.Standard.E4.Flex (4 OCPU): 2 VNICs
   # VM.Standard.E4.Flex (8 OCPU): 4 VNICs
   # BM.Standard.E4.128: 24 VNICs
   ```

2. **选项 B**: 添加更多节点到集群
   ```bash
   # 水平扩展而非垂直扩展
   kubectl scale deployment <your-app> --replicas=<desired>
   ```

3. **选项 C**: 增加每 VNIC 的 IP 数（已经是最大值 32）

**预防**: 在 Helm values 中设置合适的限制
```yaml
oci:
  maxVNICsPerNode: 2  # 根据形状调整
```

---

### 错误 3: "Permission denied: not authorized to manage VNICs"

**症状**: Operator 日志中显示权限错误

**原因**: 缺少 IAM 权限或实例主体配置不正确

**诊断**:
```bash
# 检查认证方法
kubectl -n kube-system get cm cilium-config -o yaml | grep -i instance

# 检查 operator 日志中的认证错误
kubectl -n kube-system logs deployment/cilium-operator | grep -i "not authorized\|permission denied"

# 验证实例主体
# 从节点内部运行:
curl -H "Authorization: Bearer Oracle" \
  http://169.254.169.254/opc/v2/instance/region
```

**解决方案**:

1. **选项 A**: 修复实例主体（推荐）
   
   a) 创建动态组:
   ```hcl
   # 在 OCI 控制台中 Identity & Security → Dynamic Groups
   # 规则:
   ANY {instance.compartment.id = 'ocid1.compartment.oc1..xxx'}
   ```
   
   b) 创建策略:
   ```hcl
   # 在 OCI 控制台中 Identity & Security → Policies
   Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <name>
   Allow dynamic-group cilium-oci-ipam to use subnets in compartment <name>
   Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <name>
   Allow dynamic-group cilium-oci-ipam to use virtual-network-family in compartment <name>
   Allow dynamic-group cilium-oci-ipam to read instance-family in compartment <name>
   ```

2. **选项 B**: 使用配置文件认证
   ```yaml
   # 在 Helm values 中:
   OCIUseInstancePrincipal: false
   
   oci:
     configPath: "/root/.oci/config"
   ```
   
   然后在每个节点上创建配置:
   ```bash
   mkdir -p /root/.oci
   cat > /root/.oci/config <<EOF
   [DEFAULT]
   user=<user-ocid>
   fingerprint=<fingerprint>
   key_file=/root/.oci/oci_api_key.pem
   tenancy=<tenancy-ocid>
   region=<region>
   EOF
   ```

---

### 错误 4: "VCN ID not found in metadata"

**症状**: Operator 日志显示无法获取 VCN ID

**原因**: OCI 实例元数据不提供 VCN ID - 必须手动指定

**诊断**:
```bash
# 检查是否已设置 VCN ID
kubectl -n kube-system get cm cilium-config -o yaml | grep vcn-id

# 检查 operator 参数
kubectl -n kube-system get deployment cilium-operator -o yaml | grep oci-vcn-id
```

**解决方案**: 在 Helm values 中明确设置 VCN ID（**必需**）
```yaml
oci:
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."

operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...
```

---

### 错误 5: "IPAM: unable to allocate IP, all VNICs full"

**症状**: 节点上所有 VNIC 都达到了 32 IP 限制

**原因**: 节点 Pod 数超过容量

**诊断**:
```bash
# 检查每个 VNIC 的 IP 数
kubectl get ciliumnode <node-name> -o json | \
  jq '.status.oci.vnics[] | {id: .id, ip_count: (.addresses | length)}'

# 计算节点容量
# 容量 = VNIC 数 × 32 IP/VNIC
kubectl get ciliumnode <node-name> -o json | \
  jq '.status.oci | {max_vnics: .["vnic-limits"]["max-vnics"], current_vnics: (.vnics | length)}'
```

**解决方案**:
1. **选项 A**: 添加更多节点
   ```bash
   # 增加 OKE 节点池大小
   # 或添加新节点到自管理集群
   ```

2. **选项 B**: 使用具有更多 VNIC 的实例形状
   ```bash
   # 迁移到裸金属或更大的 VM
   ```

3. **选项 C**: 减少每节点的 Pod 数
   ```yaml
   # 在 kubelet 配置中:
   maxPods: 60  # 基于 VNIC 限制调整
   ```

---

### 错误 6: "Secondary IP allocation failed"

**症状**: 无法向现有 VNIC 添加辅助 IP

**原因**: VNIC 达到 32 IP 限制或子网 IP 已满

**诊断**:
```bash
# 检查 VNIC 的 IP 数
oci network private-ip list --vnic-id <vnic-ocid> | jq 'length'

# 检查子网可用 IP
oci network subnet get --subnet-id <subnet-ocid> \
  --query 'data."available-ips"'
```

**解决方案**:
1. 如果 VNIC < 32 IP: 检查子网容量（参见错误 1）
2. 如果 VNIC = 32 IP: 将创建新 VNIC（正常行为）

---

## IPAM 问题

### 问题: Pod 卡在 ContainerCreating

**诊断流程**:
```bash
# 1. 检查 Pod 事件
kubectl describe pod <pod-name>

# 查找 IPAM 相关错误:
# - "failed to allocate IP"
# - "waiting for IP allocation"
# - "IPAM timeout"

# 2. 检查 CiliumNode IPAM 状态
kubectl get ciliumnode <node-name> -o yaml | grep -A 20 ipam

# 3. 检查 operator 日志
kubectl -n kube-system logs deployment/cilium-operator | grep <pod-name>

# 4. 检查可用 IP
kubectl get ciliumnodes -o json | \
  jq -r '.items[] | {name: .metadata.name, available: .status.ipam.available}'
```

**常见原因和解决方案**:

| 原因 | 症状 | 解决方案 |
|------|------|----------|
| 子网已满 | "no available subnets" | 添加更多子网 |
| VNIC 限制 | "LimitExceeded" | 使用更大的实例形状 |
| Operator 崩溃 | 无 IPAM 日志 | 重启 operator |
| 网络延迟 | 超时错误 | 检查 OCI API 延迟 |
| IAM 权限 | "not authorized" | 修复 IAM 策略 |

---

### 问题: IP 分配缓慢

**症状**: Pod 需要 30+ 秒才能获得 IP

**诊断**:
```bash
# 测量 IPAM 延迟
kubectl -n kube-system logs deployment/cilium-operator | grep "allocated IP" | tail -20

# 检查 OCI API 响应时间
time oci network vnic get --vnic-id <vnic-ocid>
```

**优化**:
```yaml
# 增加预分配
oci:
  vnicPreAllocationThreshold: 16
  maxIPsPerVNIC: 32

# 启用 IPAM 并行处理
operator:
  replicas: 2  # 仅限非 HA 集群
```

---

### 问题: IP 地址泄漏

**症状**: CiliumNode 显示已分配的 IP，但没有运行的 Pod

**诊断**:
```bash
# 比较 CiliumNode IP 和实际 Pod IP
ALLOCATED=$(kubectl get ciliumnode <node-name> -o json | \
  jq -r '.status.oci.vnics[].addresses[]' | sort)

USED=$(kubectl get pods -A -o wide --field-selector spec.nodeName=<node-name> \
  --no-headers | awk '{print $7}' | sort)

# 查找差异
comm -23 <(echo "$ALLOCATED") <(echo "$USED")
```

**解决方案**:
```bash
# 强制 IPAM 同步
kubectl delete ciliumnode <node-name>
# 将自动重新创建

# 或重启节点上的 cilium agent
kubectl -n kube-system delete pod -l k8s-app=cilium --field-selector spec.nodeName=<node-name>
```

---

## 网络问题

### 问题: Pod 无法相互通信

**诊断**:
```bash
# 1. 检查 Pod IP
kubectl get pods -A -o wide

# 2. 测试连接
kubectl exec -it <pod-1> -- ping <pod-2-ip>

# 3. 检查 Cilium 端点
kubectl -n kube-system exec -it ds/cilium -- cilium endpoint list

# 4. 检查路由
kubectl -n kube-system exec -it ds/cilium -- ip route

# 5. 检查 OCI 安全列表
# 在 OCI 控制台中验证子网安全列表允许 Pod CIDR
```

**常见问题**:
1. **安全列表阻止**: 在子网安全列表中添加入站规则
   ```
   Source: 10.0.0.0/16 (VCN CIDR)
   Protocol: All
   ```

2. **路由问题**: 验证 autoDirectNodeRoutes
   ```yaml
   autoDirectNodeRoutes: true
   tunnel: disabled
   ```

3. **Cilium 策略**: 检查 CiliumNetworkPolicy
   ```bash
   kubectl get cnp -A
   ```

---

### 问题: Pod 无法访问互联网

**诊断**:
```bash
# 测试 DNS
kubectl exec -it <pod-name> -- nslookup google.com

# 测试外部 IP
kubectl exec -it <pod-name> -- ping 8.8.8.8

# 测试 HTTPS
kubectl exec -it <pod-name> -- curl -I https://www.google.com
```

**检查清单**:
- [ ] VCN 有互联网网关？
- [ ] 子网路由表包括默认路由？
- [ ] 启用伪装？
- [ ] NAT 网关（用于私有子网）？

**解决方案**:
```yaml
# 启用伪装（应该默认启用）
enableIPv4Masquerade: true
ipv4NativeRoutingCIDR: "10.0.0.0/16"  # VCN CIDR
```

---

### 问题: NodePort 服务无法访问

**诊断**:
```bash
# 检查服务
kubectl get svc <service-name>

# 测试从节点访问
kubectl get nodes -o wide
curl http://<node-ip>:<node-port>

# 检查 iptables 规则
kubectl -n kube-system exec -it ds/cilium -- iptables -t nat -L -n | grep <node-port>
```

**常见问题**:
1. **OCI 安全列表**: 打开 NodePort 范围 (30000-32767)
2. **防火墙**: 在节点上禁用 firewalld
   ```bash
   sudo systemctl stop firewalld
   sudo systemctl disable firewalld
   ```

---

## 性能问题

### 问题: Pod 网络吞吐量低

**基准测试**:
```bash
# 部署 iperf3
kubectl create deployment iperf3-server --image=networkstatic/iperf3 -- iperf3 -s
kubectl create deployment iperf3-client --image=networkstatic/iperf3

# 获取服务器 Pod IP
SERVER_IP=$(kubectl get pod -l app=iperf3-server -o jsonpath='{.items[0].status.podIP}')

# 运行测试
kubectl exec -it deployment/iperf3-client -- iperf3 -c $SERVER_IP -t 30

# 应该看到 > 1 Gbps 对于同一节点
# 应该看到 > 500 Mbps 对于跨节点
```

**优化**:
```yaml
# 启用 BPF 主机路由
bpf:
  hostRouting: true

# 调整 MTU
mtu: 9000  # 对于支持巨型帧的 OCI
```

---

### 问题: 高 IPAM 延迟

**测量**:
```bash
# 创建测试部署
kubectl create deployment latency-test --image=nginx --replicas=10

# 观察 Pod 启动时间
kubectl get events --sort-by='.lastTimestamp' | grep latency-test

# 检查 IPAM 分配时间
kubectl -n kube-system logs deployment/cilium-operator | \
  grep "allocated IP" | \
  awk '{print $1, $2, $(NF-2), $(NF-1), $NF}'
```

**优化策略**:
```yaml
oci:
  # 激进的预分配
  vnicPreAllocationThreshold: 32
  
  # 并行 VNIC 创建
  maxParallelAllocations: 5
  
# 增加 operator 副本（非 HA）
operator:
  replicas: 2
```

---

## 配置问题

### 问题: Helm 升级失败

**症状**: `helm upgrade` 失败并显示验证错误

**诊断**:
```bash
# 检查当前值
helm get values cilium -n kube-system

# 验证新值
helm template cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values new-values.yaml \
  --debug
```

**常见错误**:
```yaml
# ❌ 错误: 缺少 VCN ID
ipam:
  mode: "oci"
# operator 将失败！

# ✅ 正确
ipam:
  mode: "oci"
oci:
  vcnId: "ocid1.vcn.oc1.phx.xxx"
operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
```

---

### 问题: ConfigMap 不一致

**诊断**:
```bash
# 比较 ConfigMap 和 Helm 值
kubectl -n kube-system get cm cilium-config -o yaml > current-config.yaml
helm get values cilium -n kube-system > helm-values.yaml

# 手动比较或使用 diff
diff -u <(kubectl -n kube-system get cm cilium-config -o yaml | grep -A 50 data:) \
        <(helm template cilium cilium/cilium --values helm-values.yaml | grep -A 50 data:)
```

**解决方案**:
```bash
# 强制重新创建 ConfigMap
helm upgrade cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values values.yaml \
  --force

# 重启 Cilium Pod
kubectl -n kube-system rollout restart ds/cilium
kubectl -n kube-system rollout restart deployment/cilium-operator
```

---

## 高级调试

### 启用调试日志

```bash
# 临时启用（重启后丢失）
kubectl -n kube-system exec -it ds/cilium -- cilium config Debug=true

# 永久启用（通过 Helm）
helm upgrade cilium cilium/cilium \
  --reuse-values \
  --set debug.enabled=true

# 启用特定子系统
kubectl -n kube-system exec -it ds/cilium -- cilium config DebugVerbose=flow,kvstore,envoy
```

### BPF 映射检查

```bash
# 列出所有 BPF 映射
kubectl -n kube-system exec -it ds/cilium -- cilium bpf map list

# 检查端点映射
kubectl -n kube-system exec -it ds/cilium -- cilium bpf endpoint list

# 检查 IPAM 映射
kubectl -n kube-system exec -it ds/cilium -- cilium bpf ipam list
```

### 数据包捕获

```bash
# 在特定 Pod 接口上捕获
kubectl -n kube-system exec -it ds/cilium -- \
  tcpdump -i cilium_host -w /tmp/capture.pcap

# 从容器复制 pcap
kubectl -n kube-system cp cilium-xxxxx:/tmp/capture.pcap ./capture.pcap

# 使用 Wireshark 分析
wireshark capture.pcap
```

### Cilium Monitor

```bash
# 实时监控所有事件
kubectl -n kube-system exec -it ds/cilium -- cilium monitor

# 过滤特定事件
kubectl -n kube-system exec -it ds/cilium -- cilium monitor --type drop
kubectl -n kube-system exec -it ds/cilium -- cilium monitor --type trace --from <pod-ip>
```

### 策略故障排查

```bash
# 检查端点策略
kubectl -n kube-system exec -it ds/cilium -- cilium endpoint list

# 获取特定端点的策略
kubectl -n kube-system exec -it ds/cilium -- cilium endpoint get <endpoint-id>

# 检查策略执行
kubectl -n kube-system exec -it ds/cilium -- cilium policy get
```

---

## OCI 特定调试

### 验证实例元数据

```bash
# 从节点内部
curl http://169.254.169.254/opc/v2/instance/ | jq

# 检查关键字段
curl http://169.254.169.254/opc/v2/instance/region
curl http://169.254.169.254/opc/v2/instance/compartmentId
curl http://169.254.169.254/opc/v2/instance/shape
curl http://169.254.169.254/opc/v2/instance/availabilityDomain
```

### 检查 VNIC 状态

```bash
# 使用 OCI CLI
oci network vnic get --vnic-id <vnic-ocid> | jq

# 检查关键字段
oci network vnic get --vnic-id <vnic-ocid> | \
  jq '{
    id: .data.id,
    state: .data."lifecycle-state",
    primary: .data."is-primary",
    subnet: .data."subnet-id",
    private_ip: .data."private-ip",
    public_ip: .data."public-ip"
  }'

# 列出 VNIC 的私有 IP
oci network private-ip list --vnic-id <vnic-ocid> | \
  jq -r '.data[] | {ip: ."ip-address", primary: ."is-primary"}'
```

### 审计 OCI API 调用

```bash
# 启用 OCI 审计
# 在 OCI 控制台中: Observability → Audit → 创建审计配置

# 搜索 Cilium 的 API 调用
# 服务: Virtual Networking
# 操作: CreateVnic, AttachVnic, CreatePrivateIp, DeletePrivateIp

# 使用 OCI CLI 查询审计事件
oci audit event list \
  --compartment-id <compartment-ocid> \
  --start-time "2024-01-01T00:00:00.000Z" \
  --end-time "2024-01-02T00:00:00.000Z" \
  --query 'data[?contains("event-name", `Vnic`)]'
```

---

## 收集支持包

如果需要打开支持工单:

```bash
#!/bin/bash
# collect-cilium-oci-debug.sh

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_DIR="cilium-oci-debug-${TIMESTAMP}"
mkdir -p "$OUTPUT_DIR"

# Cilium 状态
kubectl -n kube-system get pods > "$OUTPUT_DIR/pods.txt"
kubectl -n kube-system logs deployment/cilium-operator > "$OUTPUT_DIR/operator.log"
kubectl -n kube-system logs ds/cilium --all-containers > "$OUTPUT_DIR/agent.log"

# 配置
kubectl -n kube-system get cm cilium-config -o yaml > "$OUTPUT_DIR/config.yaml"
helm get values cilium -n kube-system > "$OUTPUT_DIR/helm-values.yaml"

# IPAM 状态
kubectl get ciliumnodes -o yaml > "$OUTPUT_DIR/ciliumnodes.yaml"

# 节点信息
kubectl get nodes -o wide > "$OUTPUT_DIR/nodes.txt"
kubectl describe nodes > "$OUTPUT_DIR/nodes-describe.txt"

# Pod 信息
kubectl get pods -A -o wide > "$OUTPUT_DIR/pods-all.txt"

# 事件
kubectl get events -A --sort-by='.lastTimestamp' > "$OUTPUT_DIR/events.txt"

# OCI 信息（如果可用）
if command -v oci &> /dev/null; then
  oci compute instance list --compartment-id <compartment-ocid> > "$OUTPUT_DIR/oci-instances.json"
  oci network vcn get --vcn-id <vcn-ocid> > "$OUTPUT_DIR/oci-vcn.json"
  oci network subnet list --vcn-id <vcn-ocid> --compartment-id <compartment-ocid> > "$OUTPUT_DIR/oci-subnets.json"
fi

# 打包
tar czf "${OUTPUT_DIR}.tar.gz" "$OUTPUT_DIR"
echo "Debug package created: ${OUTPUT_DIR}.tar.gz"
```

---

## 快速故障排查检查清单

从这些命令开始:

```bash
# ✅ Cilium 健康
kubectl -n kube-system get pods -l k8s-app=cilium
kubectl -n kube-system exec -it ds/cilium -- cilium status

# ✅ IPAM 配置
kubectl -n kube-system get cm cilium-config -o yaml | grep -E "ipam|oci"

# ✅ IPAM 状态
kubectl get ciliumnodes -o wide

# ✅ Operator 日志
kubectl -n kube-system logs deployment/cilium-operator --tail=50

# ✅ Pod 网络
kubectl get pods -A -o wide
kubectl exec -it <pod-name> -- ping <another-pod-ip>

# ✅ OCI 权限
kubectl -n kube-system logs deployment/cilium-operator | grep -i "not authorized\|permission"

# ✅ 子网容量
kubectl get ciliumnodes -o json | \
  jq -r '.items[] | {name: .metadata.name, vnics: (.status.oci.vnics | length)}'
```

---

## 获取帮助

如果您仍然遇到问题:

1. 📖 查看 [配置参考](configuration_CN.md)
2. 📖 查看 [README](README_CN.md)
3. 📖 查看 [快速入门](quickstart_CN.md)
4. 🌐 查阅 [English Documentation](troubleshooting.md)
5. 🐛 提交 GitHub issue 并附带日志
6. 💬 在 Cilium Slack 中寻求帮助

---

## 已知限制

1. **VCN ID 必需**: 无法从元数据自动检测
2. **VNIC 限制**: 受实例形状约束
3. **每 VNIC 32 IP**: OCI 硬限制
4. **无热附加**: VNIC 附加需要 ~5-10 秒
5. **子网锁定**: Pod 无法在 VNIC 创建后切换子网

## 最佳实践

- ✅ 始终设置 `oci.vcnId` 和 `--oci-vcn-id`
- ✅ 使用实例主体而非配置文件
- ✅ 为子网规划足够的 IP 空间
- ✅ 根据 VNIC 限制监控节点容量
- ✅ 使用多个子网以实现冗余
- ✅ 为 OCI API 调用启用审计日志
- ✅ 定期检查子网 IP 可用性
- ✅ 使用 Hubble 进行网络可观察性
