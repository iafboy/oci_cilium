# OCI IPAM 快速入门指南

通过 5 个简单的步骤在 OCI 上使用 Cilium OCI IPAM。

## 前提条件

在开始之前，请确保您具备:

- ✅ 运行在 OCI 上的 Kubernetes 集群（OKE 或自管理）
- ✅ Kubernetes 版本 1.23+
- ✅ 已安装 `kubectl` 和 `helm`
- ✅ 具有适当 IAM 策略的 OCI 实例主体或配置文件
- ✅ 具有足够 IP 空间的 OCI VCN

## 第 1 步: 准备 OCI 环境

### 1.1 获取 VCN OCID

```bash
# 方法 1: 使用 OCI 控制台
# - 导航到 Networking → Virtual Cloud Networks
# - 点击您的 VCN
# - 从详细信息页面复制 OCID

# 方法 2: 使用 OCI CLI
oci network vcn list \
  --compartment-id <your-compartment-ocid> \
  --display-name <your-vcn-name> \
  --query 'data[0].id' \
  --raw-output
```

示例输出:
```
ocid1.vcn.oc1.phx.aaaaaaaa...
```

### 1.2 验证子网配置

检查您的 VCN 子网是否有足够的 IP:

```bash
# 列出 VCN 中的子网
oci network subnet list \
  --compartment-id <compartment-ocid> \
  --vcn-id <vcn-ocid> \
  --query 'data[*].{Name:"display-name", CIDR:"cidr-block", AvailableIPs:"available-ips"}' \
  --output table
```

**建议**: 确保每个子网至少有 100+ 个可用 IP 用于 Pod 分配。

### 1.3 设置 IAM 策略

创建动态组（用于实例主体认证）:

```bash
# 创建包含集群节点的动态组
# 规则示例:
ANY {instance.compartment.id = 'ocid1.compartment.oc1..xxx'}
```

创建 IAM 策略以授予 VNIC 管理权限:

```hcl
# 策略语句
Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use subnets in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use virtual-network-family in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to read instance-family in compartment <compartment-name>
```

在 OCI 控制台中应用:
1. 导航到 Identity & Security → Policies
2. 点击 "Create Policy"
3. 添加上述语句
4. 选择正确的区间

### 1.4 （可选）配置文件方法

如果不使用实例主体:

```bash
# 在每个节点上创建 OCI 配置文件
mkdir -p /root/.oci
cat > /root/.oci/config <<EOF
[DEFAULT]
user=<user-ocid>
fingerprint=<api-key-fingerprint>
key_file=/root/.oci/oci_api_key.pem
tenancy=<tenancy-ocid>
region=<region-identifier>
EOF

# 复制您的 API 私钥
cp ~/path/to/your/private_key.pem /root/.oci/oci_api_key.pem
chmod 600 /root/.oci/oci_api_key.pem
```

## 第 2 步: 创建 Helm Values 文件

创建 `cilium-oci-values.yaml`:

```yaml
# ============================================
# 必需的 OCI IPAM 配置
# ============================================

ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."  # 替换为您的 VCN OCID

# 使用实例主体认证（推荐）
OCIUseInstancePrincipal: true

operator:
  replicas: 1
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...  # 替换为您的 VCN OCID

# ============================================
# 推荐的生产配置
# ============================================

# 启用 Hubble 可观察性
hubble:
  enabled: true
  relay:
    enabled: true
  ui:
    enabled: true

# 网络配置
tunnel: disabled
autoDirectNodeRoutes: true
ipv4NativeRoutingCIDR: "10.0.0.0/16"  # 替换为您的 VCN CIDR

# 启用 Bandwidth Manager（可选）
bandwidthManager:
  enabled: false

# 安全配置
policyEnforcementMode: "default"

# ============================================
# 可选的高级配置
# ============================================

# 如果不使用实例主体:
# OCIUseInstancePrincipal: false
# oci:
#   configPath: "/root/.oci/config"

# 子网选择（可选）- 通过标签过滤子网
# oci:
#   subnetTags:
#     environment: production
#     tier: app

# 预分配设置
# oci:
#   vnicPreAllocationThreshold: 8
#   maxIPsPerVNIC: 32
```

### 关键配置说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `ipam.mode` | **必需**: 必须设置为 "oci" | 无 |
| `oci.vcnId` | **必需**: 您的 OCI VCN OCID | 无 |
| `OCIUseInstancePrincipal` | 使用实例主体而非配置文件 | true |
| `operator.extraArgs[--oci-vcn-id]` | **必需**: Operator 需要 VCN OCID | 无 |
| `tunnel` | 应设置为 "disabled" 以实现原生路由 | vxlan |
| `autoDirectNodeRoutes` | 在节点之间启用直接路由 | false |
| `ipv4NativeRoutingCIDR` | 应匹配您的 VCN CIDR | 无 |

## 第 3 步: 安装 Cilium

### 3.1 添加 Cilium Helm 仓库

```bash
helm repo add cilium https://helm.cilium.io/
helm repo update
```

### 3.2 安装 Cilium

```bash
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values cilium-oci-values.yaml
```

### 3.3 等待部署

```bash
# 观察 Pod 启动
kubectl -n kube-system get pods -l k8s-app=cilium -w

# 应该看到:
# NAME                               READY   STATUS    RESTARTS   AGE
# cilium-operator-xxxxxxxxxx-xxxxx   1/1     Running   0          30s
# cilium-xxxxx                       1/1     Running   0          30s
# cilium-yyyyy                       1/1     Running   0          30s
```

等待所有 Cilium Pod 达到 `Running` 状态（通常需要 1-2 分钟）。

## 第 4 步: 验证安装

### 4.1 检查 Cilium 状态

```bash
# 使用 Cilium CLI（如果已安装）
cilium status --wait

# 或使用 kubectl
kubectl -n kube-system exec -it ds/cilium -- cilium status
```

预期输出:
```
KVStore:                 Ok   etcd: 1/1 connected, has-quorum=true
Kubernetes:              Ok   1.25 (v1.25.7) [linux/amd64]
Kubernetes APIs:         ["cilium/v2::CiliumClusterwideNetworkPolicy", ...]
Cilium:                  Ok   1.15.2
NodeMonitor:             Listening for events on 2 CPUs with 64x4096 of shared memory
Cilium health daemon:    Ok
IPAM:                    IPv4: 5/254 allocated from 10.0.1.0/24
```

### 4.2 验证 OCI IPAM 模式

```bash
# 检查 Cilium ConfigMap
kubectl -n kube-system get cm cilium-config -o yaml | grep -A 5 ipam

# 应该看到:
# ipam: oci
# oci-vcn-id: ocid1.vcn.oc1.phx.aaaaaa...
```

### 4.3 检查 CiliumNode 资源

```bash
# 列出 CiliumNode
kubectl get ciliumnodes

# 获取详细的 OCI 状态
kubectl get ciliumnode <node-name> -o yaml
```

您应该在 `status.oci.vnics` 下看到 OCI VNIC 信息:

```yaml
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
        subnet:
          id: "ocid1.subnet.oc1.phx.xxx"
          cidr: "10.0.1.0/24"
```

## 第 5 步: 测试 Pod 网络

### 5.1 部署测试 Pod

```bash
# 创建测试部署
kubectl create deployment test-pod --image=nicolaka/netshoot --replicas=3 -- sleep 3600

# 等待 Pod 运行
kubectl wait --for=condition=ready pod -l app=test-pod --timeout=60s
```

### 5.2 验证 Pod IP 分配

```bash
# 获取 Pod IP
kubectl get pods -l app=test-pod -o wide

# 应该看到 OCI VCN 子网范围内的 IP:
# NAME                        READY   STATUS    IP          NODE
# test-pod-xxxxxxxxxx-xxxxx   1/1     Running   10.0.1.10   node-1
# test-pod-xxxxxxxxxx-yyyyy   1/1     Running   10.0.1.11   node-2
# test-pod-xxxxxxxxxx-zzzzz   1/1     Running   10.0.2.10   node-3
```

**验证**: Pod IP 应该在您的 VCN 子网范围内（例如 10.0.x.x）。

### 5.3 测试 Pod 到 Pod 连接

```bash
# 从一个 Pod ping 另一个
POD1=$(kubectl get pod -l app=test-pod -o jsonpath='{.items[0].metadata.name}')
POD2=$(kubectl get pod -l app=test-pod -o jsonpath='{.items[1].metadata.name}')
POD2_IP=$(kubectl get pod $POD2 -o jsonpath='{.status.podIP}')

kubectl exec -it $POD1 -- ping -c 3 $POD2_IP

# 应该看到成功的 ping 响应
```

### 5.4 测试 Pod 到外部连接

```bash
# 测试互联网连接
kubectl exec -it $POD1 -- curl -I https://www.google.com

# 应该看到 HTTP 响应头
```

### 5.5 验证 OCI VNIC 创建

在 OCI 控制台中:
1. 导航到 Compute → Instances
2. 选择一个 Kubernetes 节点
3. 点击 "Attached VNICs"
4. 验证是否已创建辅助 VNIC（如果 Pod 超过主 VNIC 容量）

或使用 OCI CLI:

```bash
# 列出实例的 VNIC
oci compute vnic-attachment list \
  --instance-id <instance-ocid> \
  --compartment-id <compartment-ocid>
```

## 成功！🎉

您的 Cilium OCI IPAM 集成现已运行！

## 下一步

### 配置优化

- 📖 阅读 [configuration_CN.md](configuration_CN.md) 以了解高级选项
- 🔧 调整预分配阈值以优化性能
- 🏷️ 使用子网标签进行智能 Pod 放置

### 监控和可观察性

```bash
# 检查 Cilium Operator 日志
kubectl -n kube-system logs deployment/cilium-operator

# 检查 IPAM 事件
kubectl get events --all-namespaces | grep -i ipam

# 监控 VNIC 使用
kubectl get ciliumnodes -o json | \
  jq -r '.items[] | {name: .metadata.name, vnics: (.status.oci.vnics | length)}'
```

### 启用 Hubble UI（可观察性）

```bash
# 端口转发 Hubble UI
kubectl -n kube-system port-forward svc/hubble-ui 8080:80

# 在浏览器中打开
# http://localhost:8080
```

### 性能测试

```bash
# 安装测试工具
kubectl create deployment perf-test --image=networkstatic/iperf3 --replicas=2

# 运行性能测试
POD1=$(kubectl get pod -l app=perf-test -o jsonpath='{.items[0].metadata.name}')
POD2=$(kubectl get pod -l app=perf-test -o jsonpath='{.items[1].metadata.name}')

# 在一个终端中启动 iperf3 服务器
kubectl exec -it $POD1 -- iperf3 -s

# 在另一个终端中启动客户端
kubectl exec -it $POD2 -- iperf3 -c <pod1-ip> -t 10
```

## 常见问题

### Pod 启动缓慢

**原因**: 首次创建 VNIC 需要时间（3-5 秒）

**解决方案**: 
```yaml
# 增加预分配阈值
oci:
  vnicPreAllocationThreshold: 16
```

### IP 耗尽错误

**原因**: 子网没有可用 IP

**解决方案**:
1. 向 VCN 添加更多子网
2. 使用更大的 CIDR 块
3. 检查未使用的 IP 分配

### VNIC 附加失败

**原因**: 实例形状达到 VNIC 限制

**解决方案**:
```bash
# 检查实例形状限制
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnic-limits}'

# 添加更多节点或使用更大的实例形状
```

### 权限被拒绝错误

**原因**: 缺少 IAM 权限

**解决方案**: 验证实例主体动态组和策略（参见第 1.3 步）

## 故障排查

如果出现问题:

1. **检查日志**:
   ```bash
   kubectl -n kube-system logs deployment/cilium-operator
   kubectl -n kube-system logs ds/cilium
   ```

2. **验证配置**:
   ```bash
   kubectl -n kube-system get cm cilium-config -o yaml
   ```

3. **检查 IPAM 状态**:
   ```bash
   kubectl get ciliumnodes -o yaml
   ```

4. **查阅完整的故障排查指南**: [troubleshooting_CN.md](troubleshooting_CN.md)

## 清理

如果您想移除测试部署:

```bash
# 删除测试 Pod
kubectl delete deployment test-pod
kubectl delete deployment perf-test

# 卸载 Cilium（谨慎！）
helm uninstall cilium -n kube-system
```

## 获取帮助

- 📖 [配置参考](configuration_CN.md)
- 🔧 [故障排查指南](troubleshooting_CN.md)
- 📚 [主 README](README_CN.md)
- 🌐 [English Documentation](quickstart.md)

祝您在 OCI 上愉快地使用 Cilium！🐝
