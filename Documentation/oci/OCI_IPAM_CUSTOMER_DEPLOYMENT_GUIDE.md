# Cilium OCI IPAM 客户部署指南

**面向用户**: 运维工程师、DevOps团队  
**前置要求**: 基本的Kubernetes和OCI知识  
**预计时间**: 2-3小时（首次部署）  
**版本**: Cilium v1.15.2  
**最后更新**: 2025年10月27日

---

## 📋 快速导航

- [部署前准备](#部署前准备)
- [快速部署（推荐）](#快速部署推荐)
- [详细部署步骤](#详细部署步骤)
- [功能验证](#功能验证)
- [常见问题](#常见问题)
- [故障排查](#故障排查)

---

## 部署前准备

### 1. 环境要求

#### Kubernetes集群

| 要求 | 说明 |
|------|------|
| **K8s版本** | 1.21+ |
| **节点数量** | 最少3个（1 master + 2 workers） |
| **节点规格** | 推荐 VM.Standard.E5.Flex (4 OCPU, 16GB RAM) |
| **操作系统** | Oracle Linux 8.x 或 Ubuntu 20.04+ |

#### OCI资源

| 资源 | OCID示例 | 获取方式 |
|------|----------|----------|
| **VCN** | `ocid1.vcn.oc1.region...` | OCI控制台 → Networking → VCNs |
| **Compartment** | `ocid1.compartment.oc1...` | OCI控制台 → Identity → Compartments |
| **Subnets** | `ocid1.subnet.oc1...` | VCN详情页面 |

### 2. 收集必要信息

填写以下信息表格，部署时需要：

```yaml
# 保存为 deployment-info.txt

# VCN信息
VCN_OCID="ocid1.vcn.oc1.ap-singapore-2.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"
VCN_CIDR="10.0.0.0/16"

# Compartment信息
COMPARTMENT_OCID="ocid1.compartment.oc1..aaaaaaaa..."

# Pod网络Subnet
POD_SUBNET_1="10.0.1.0/24"
POD_SUBNET_2="10.0.2.0/24"
POD_SUBNET_3="10.0.3.0/24"

# Subnet Tag
SUBNET_TAG_KEY="cilium-pod-network"
SUBNET_TAG_VALUE="yes"
```

### 3. 配置IAM权限（重要！）

#### 步骤1: 创建Dynamic Group

登录OCI控制台 → Identity → Dynamic Groups → Create Dynamic Group

**名称**: `cilium-oci-ipam`

**规则**:
```
# 方式1: 匹配Compartment（推荐）
instance.compartment.id = 'ocid1.compartment.oc1..aaaaaaaa...'

# 方式2: 匹配特定实例
matching_instance_id = 'ocid1.instance.oc1.ap-singapore-2.anzxsljrqakycgfd...'
```

#### 步骤2: 创建Policy

登录OCI控制台 → Identity → Policies → Create Policy

**名称**: `cilium-oci-ipam-policy`  
**Compartment**: 选择VCN所在的Compartment

**Policy Statements**:
```
Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use subnets in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <compartment-name>
Allow dynamic-group cilium-oci-ipam to use private-ips in compartment <compartment-name>
```

#### 步骤3: 验证权限

在任意K8s节点上运行：

```bash
# 测试Instance Principal
oci iam region list --auth instance_principal

# 应该看到region列表，而不是权限错误
```

✅ **如果成功显示region列表，说明IAM配置正确！**

---

## 快速部署（推荐）

### 方案A: 使用Subnet Tags自动VNIC创建 ⭐ 推荐

**优势**: 完全自动化，无需手动创建VNIC

#### 1. 为Subnet添加Tags

```bash
# 设置变量
export SUBNET_1_OCID="ocid1.subnet.oc1..."
export SUBNET_2_OCID="ocid1.subnet.oc1..."
export SUBNET_3_OCID="ocid1.subnet.oc1..."

# 批量添加Tag
for subnet in $SUBNET_1_OCID $SUBNET_2_OCID $SUBNET_3_OCID; do
  oci network subnet update \
    --subnet-id $subnet \
    --freeform-tags '{"cilium-pod-network":"yes"}' \
    --force \
    --auth instance_principal
done
```

验证：
```bash
oci network subnet get \
  --subnet-id $SUBNET_1_OCID \
  --query 'data."freeform-tags"' \
  --auth instance_principal

# 应该看到: {"cilium-pod-network": "yes"}
```

#### 2. 准备Helm Values文件

```bash
cat > cilium-oci-values.yaml <<'EOF'
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - "10.0.0.0/16"  # 替换为您的VCN CIDR

oci:
  enabled: true
  useInstancePrincipal: true
  vcnID: "ocid1.vcn.oc1.ap-singapore-2.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq"  # 替换为您的VCN OCID
  
  # Subnet Tags配置（配置1）
  subnetTags:
    cilium-pod-network: "yes"
  
  # VNIC管理参数
  vnicPreAllocationThreshold: 0.8
  maxIPsPerVNIC: 32

operator:
  replicas: 2
  
  # ⚠️ 关键：必须显式配置（配置2）
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.ap-singapore-2.amaaaaaaak7gbriaqakycgfd5ot4lqbbotqsqb6erjznqrqma633t5zvr3mq  # 替换
    - --oci-use-instance-principal=true
    - --subnet-tags-filter=cilium-pod-network=yes  # 与subnetTags一致

# 推荐：启用监控
prometheus:
  enabled: true

# 推荐：启用Hubble
hubble:
  enabled: true
  relay:
    enabled: true
  ui:
    enabled: true
EOF
```

⚠️ **重要**: 修改以下内容为您的实际值：
- `vcnID`: 您的VCN OCID
- `clusterPoolIPv4PodCIDRList`: 您的VCN CIDR
- `--oci-vcn-id`: 与vcnID相同

#### 3. 安装Cilium

```bash
# 添加Cilium Helm仓库
helm repo add cilium https://helm.cilium.io/
helm repo update

# 安装Cilium
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values cilium-oci-values.yaml

# 等待Pods启动（约2-3分钟）
kubectl wait --for=condition=ready pod -l k8s-app=cilium -n kube-system --timeout=300s
```

#### 4. 验证安装

```bash
# 检查Cilium状态
cilium status

# 检查Operator日志（验证subnet-tags-filter生效）
kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter

# 应该看到
# --subnet-tags-filter='cilium-pod-network=yes'
```

✅ **完成！现在创建Pod时，Cilium会自动从带有tag的Subnet创建VNIC。**

---

### 方案B: 手动创建VNIC（传统方式）

如果不使用Subnet Tags，需要手动为每个节点创建VNIC。

#### 1. 创建VNIC

```bash
# 为cilium-w1节点创建VNIC
oci compute vnic-attachment attach \
  --instance-id ocid1.instance.oc1...cilium-w1... \
  --subnet-id ocid1.subnet.oc1...pod-subnet-1... \
  --display-name "cilium-w1-vnic2" \
  --auth instance_principal

# 记录返回的VNIC OCID
VNIC_OCID="ocid1.vnic.oc1..."
```

#### 2. 更新CiliumNode

```bash
kubectl edit ciliumnode cilium-w1
```

添加VNIC信息：
```yaml
spec:
  oci:
    vnics:
      ocid1.vnic.oc1...(新VNIC的OCID):
        subnet:
          cidr: "10.0.1.0/24"
          ocid: "ocid1.subnet.oc1..."
```

---

## 详细部署步骤

### 步骤1: 规划Subnet

#### 推荐配置（生产环境）

| Subnet用途 | CIDR | 可用IP | 说明 |
|-----------|------|--------|------|
| 节点主网络 | 10.0.0.0/24 | 251 | 节点主VNIC |
| Pod网络1 | 10.0.1.0/24 | 251 | AD1 Pod Subnet |
| Pod网络2 | 10.0.2.0/24 | 251 | AD2 Pod Subnet |
| Pod网络3 | 10.0.3.0/24 | 251 | AD3 Pod Subnet |

⚠️ **重要建议**:
- ✅ 使用 **/24 或更大** 的Subnet（提供250+ IP）
- ❌ 避免使用 **/28** Subnet（只有13个可用IP，太容易耗尽）
- ✅ 为每个可用域(AD)创建一个Subnet（高可用）

#### 创建Subnet（如果还没有）

```bash
# 设置变量
VCN_OCID="ocid1.vcn.oc1..."
COMPARTMENT_OCID="ocid1.compartment.oc1..."

# 创建Pod Subnet 1 (AD1)
oci network subnet create \
  --compartment-id $COMPARTMENT_OCID \
  --vcn-id $VCN_OCID \
  --cidr-block "10.0.1.0/24" \
  --display-name "cilium-pod-subnet-ad1" \
  --availability-domain "AD-1" \
  --dns-label "podnet1" \
  --route-table-id <route-table-ocid> \
  --freeform-tags '{"cilium-pod-network":"yes"}' \
  --auth instance_principal

# 记录返回的Subnet OCID
```

### 步骤2: 构建Docker镜像（可选）

如果需要自定义镜像：

```bash
# Clone代码
git clone https://github.com/iafboy/oci_cilium.git
cd oci_cilium

# 检出正确的分支
git checkout feature/oci-fork

# 构建镜像（包含OCI IPAM）
make GOFLAGS="-tags=ipam_provider_oci" docker-cilium-image
make GOFLAGS="-tags=ipam_provider_oci" docker-operator-generic-image

# 推送到您的Registry
docker tag cilium/cilium:latest your-registry/cilium:oci-v1.15.2
docker push your-registry/cilium:oci-v1.15.2
```

或者使用预构建镜像：
```yaml
# 在cilium-oci-values.yaml中
image:
  repository: "your-registry/cilium"
  tag: "oci-v1.15.2"
  useDigest: false
```

### 步骤3: 部署Cilium

见上面的"快速部署"章节。

### 步骤4: 配置Hubble（可选但推荐）

Hubble提供网络可观测性。

```bash
# 如果之前没有启用，升级启用Hubble
helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  --reuse-values \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true

# 暴露Hubble UI
kubectl port-forward -n kube-system svc/hubble-ui 12000:80

# 在浏览器访问 http://localhost:12000
```

---

## 功能验证

### 验证清单 ✓

#### 1. 验证Cilium Agent启动

```bash
kubectl get pods -n kube-system -l k8s-app=cilium

# 期望输出：所有Pods Running
# cilium-abcde   1/1     Running   0          5m
# cilium-fghij   1/1     Running   0          5m
```

#### 2. 验证Operator启动

```bash
kubectl get pods -n kube-system -l name=cilium-operator

# 期望输出：2个Pods Running（如果配置了replicas: 2）
# cilium-operator-12345   1/1     Running   0          5m
# cilium-operator-67890   1/1     Running   0          5m
```

#### 3. 验证CiliumNode状态

```bash
kubectl get ciliumnode

# 期望输出
# NAME         AGE
# cilium-m     10m
# cilium-w1    10m
# cilium-w2    10m

# 查看详细信息
kubectl get ciliumnode cilium-w1 -o yaml
```

检查关键字段：
```yaml
status:
  oci:
    vcnID: "ocid1.vcn..."
    vnics:
      ocid1.vnic.oc1...(主VNIC):
        subnet:
          cidr: "10.0.0.0/24"
        allocated-ips: 3
        available-ips: 29
```

#### 4. 验证Subnet Tags配置（如果使用）

```bash
# 检查Operator日志
kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter

# ✅ 应该看到
# --subnet-tags-filter='cilium-pod-network=yes'

# ❌ 如果看到空的，说明配置有问题
# --subnet-tags-filter=''
```

#### 5. 创建测试Pod

```bash
# 创建测试Deployment
kubectl create deployment test-nginx --image=nginx --replicas=3

# 等待Pods运行
kubectl wait --for=condition=ready pod -l app=test-nginx --timeout=60s

# 检查Pod IP（应该来自VCN subnet）
kubectl get pods -l app=test-nginx -o wide

# 期望输出：IP地址在10.0.x.x范围内
# NAME                         READY   STATUS    IP          NODE
# test-nginx-xxx-yyy           1/1     Running   10.0.1.5    cilium-w1
# test-nginx-xxx-zzz           1/1     Running   10.0.1.6    cilium-w2
```

#### 6. 验证网络连通性

```bash
# Pod to Pod (跨节点)
POD1=$(kubectl get pods -l app=test-nginx -o jsonpath='{.items[0].metadata.name}')
POD2_IP=$(kubectl get pods -l app=test-nginx -o jsonpath='{.items[1].status.podIP}')

kubectl exec $POD1 -- ping -c 3 $POD2_IP

# ✅ 期望：3 packets transmitted, 3 received, 0% packet loss
```

```bash
# Pod to Internet
kubectl exec $POD1 -- ping -c 3 8.8.8.8

# ✅ 期望：3 packets transmitted, 3 received, 0% packet loss
```

```bash
# Pod to Service
kubectl create service clusterip test-nginx --tcp=80:80

kubectl exec $POD1 -- curl -s http://test-nginx

# ✅ 期望：看到Nginx欢迎页面HTML
```

#### 7. 验证自动VNIC创建（Subnet Tags）

```bash
# 创建大量Pod触发VNIC创建
kubectl create deployment test-scale --image=busybox --replicas=50 -- sleep 3600

# 监控VNIC创建
watch kubectl get ciliumnode cilium-w1 -o jsonpath='{.status.oci.vnics}' | jq 'length'

# 期望：看到VNIC数量从2增加到3（或更多）
```

---

## 常见问题

### Q1: 为什么需要配置两处（oci.subnetTags + operator.extraArgs）？

**A**: Cilium的架构设计决定的。

- **Agent (DaemonSet)** 读取ConfigMap中的`oci.*`配置
- **Operator (Deployment)** 只读取自己的命令行参数（`operator.extraArgs`）

两个组件是独立的进程，配置不会自动传递。

**解决方案**: 必须同时配置两处：

```yaml
oci:
  subnetTags:
    cilium-pod-network: "yes"  # 配置1

operator:
  extraArgs:
    - --subnet-tags-filter=cilium-pod-network=yes  # 配置2（实际生效）
```

### Q2: 为什么推荐使用/24而不是/28 Subnet？

**A**: /28 Subnet太小，容易触发多次VNIC创建。

| Subnet大小 | 总IP | 可用IP | 说明 |
|-----------|------|--------|------|
| /28 | 16 | 13 | ❌ 太小，容易耗尽 |
| /24 | 256 | 251 | ✅ 推荐，适合生产 |
| /20 | 4096 | 4091 | ✅ 大型集群 |

**实际案例**: 在测试中，使用/28 Subnet导致创建了2个VNIC而不是1个，因为：
- Cilium surge allocation想一次分配14个IP
- /28只有13个可用IP
- 第一次失败 → 创建VNIC1
- 第二次失败 → 创建VNIC2

### Q3: 如何查看有多少个VNIC被创建？

```bash
# 方式1: 通过CiliumNode
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq 'length'

# 方式2: 通过CiliumNode详细信息
kubectl get ciliumnode <node-name> -o jsonpath='{.status.oci.vnics}' | jq -r 'keys[]'

# 方式3: 在节点上查看
ip addr show | grep "^[0-9]" | grep -v "lo:"
```

### Q4: Pod一直处于ContainerCreating状态

**可能原因**:

1. **IP地址池耗尽**
   ```bash
   # 检查VNIC IP使用情况
   kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq
   
   # 查看allocated-ips vs available-ips
   ```

2. **IAM权限不足**
   ```bash
   # 测试权限
   oci iam region list --auth instance_principal
   
   # 如果失败，检查Dynamic Group和Policy
   ```

3. **Subnet Tags配置错误**
   ```bash
   # 验证Operator配置
   kubectl logs -n kube-system deployment/cilium-operator | grep subnet-tags-filter
   
   # 应该看到 --subnet-tags-filter='xxx=yyy'，而不是空
   ```

### Q5: 如何回滚Cilium？

```bash
# 查看历史版本
helm history cilium -n kube-system

# 回滚到上一个版本
helm rollback cilium -n kube-system

# 或回滚到特定版本
helm rollback cilium 3 -n kube-system
```

### Q6: 如何升级Cilium配置？

```bash
# 方式1: 使用--reuse-values
helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  --reuse-values \
  --set newOption=newValue

# 方式2: 使用values文件
helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  -f cilium-oci-values.yaml
```

⚠️ **注意**: `--reuse-values`会保留当前所有配置，`-f`会覆盖values。

---

## 故障排查

### 诊断流程图

```
Pod无法获取IP
     │
     ├─ 检查Pod Events
     │   └─ kubectl describe pod <pod-name>
     │
     ├─ 检查CiliumNode状态
     │   └─ kubectl get ciliumnode <node> -o yaml
     │       │
     │       ├─ VNIC数量 = 0？
     │       │   └─ IAM权限问题 → 检查Dynamic Group + Policy
     │       │
     │       ├─ 所有VNIC的available-ips = 0？
     │       │   └─ IP耗尽 → 扩展Subnet或添加新VNIC
     │       │
     │       └─ VNIC状态异常？
     │           └─ OCI API问题 → 检查Operator日志
     │
     └─ 检查Operator日志
         └─ kubectl logs -n kube-system deployment/cilium-operator
             │
             ├─ "Unauthorized" / "Forbidden"
             │   └─ IAM权限问题
             │
             ├─ "subnet-tags-filter=''"
             │   └─ Subnet Tags配置错误
             │
             └─ "Unable to assign additional IPs"
                 └─ Subnet IP耗尽
```

### 常用诊断命令

```bash
# 1. 快速健康检查
cilium status

# 2. 检查所有Cilium Pods
kubectl get pods -n kube-system -l k8s-app=cilium -o wide

# 3. 检查CiliumNode状态
kubectl get ciliumnode

# 4. 查看特定节点的VNIC详情
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq

# 5. 查看Operator日志（最近100行）
kubectl logs -n kube-system deployment/cilium-operator --tail=100

# 6. 查看Agent日志（特定节点）
kubectl logs -n kube-system daemonset/cilium -c cilium-agent --tail=100 -l kubernetes.io/hostname=<node>

# 7. 检查IAM权限
oci iam region list --auth instance_principal

# 8. 检查Subnet Tags
oci network subnet get --subnet-id <subnet-ocid> --query 'data."freeform-tags"' --auth instance_principal

# 9. 运行连通性测试
cilium connectivity test

# 10. 查看Hubble flows（如果启用）
hubble observe --namespace default
```

### 日志分析关键字

| 问题类型 | 搜索关键字 | 说明 |
|---------|-----------|------|
| **IAM权限** | `Unauthorized`, `Forbidden`, `401`, `403` | 权限不足 |
| **VNIC创建** | `Unable to assign additional IPs`, `create new interface` | VNIC创建 |
| **IP分配** | `IP allocation`, `IPAM`, `no more IPs` | IP分配问题 |
| **Subnet Tags** | `subnet-tags-filter`, `matching subnet` | Subnet Tags |
| **OCI API错误** | `OCI error`, `API error`, `500` | OCI API问题 |

### 获取支持

**收集诊断信息**:

```bash
# 创建诊断目录
mkdir cilium-debug-$(date +%Y%m%d-%H%M%S)
cd cilium-debug-*

# 收集Cilium状态
cilium status > cilium-status.txt

# 收集CiliumNode
kubectl get ciliumnode -o yaml > ciliumnodes.yaml

# 收集Operator日志
kubectl logs -n kube-system deployment/cilium-operator --tail=1000 > operator.log

# 收集Agent日志
for node in $(kubectl get nodes -o jsonpath='{.items[*].metadata.name}'); do
  kubectl logs -n kube-system daemonset/cilium -c cilium-agent -l kubernetes.io/hostname=$node --tail=500 > agent-$node.log
done

# 收集Pod信息
kubectl get pods -A -o wide > all-pods.txt

# 收集Events
kubectl get events -A --sort-by='.lastTimestamp' > events.txt

# 打包
cd ..
tar czf cilium-debug-$(date +%Y%m%d-%H%M%S).tar.gz cilium-debug-*
```

**联系支持**:
- 邮箱: dengwei@xiaomi.com
- 附带上面收集的诊断包

---

## 生产环境最佳实践

### 1. Subnet规划

✅ **使用/24或更大的Subnet**  
✅ **为每个AD创建一个Subnet（高可用）**  
✅ **使用Subnet Tags管理（自动化）**  
❌ **避免使用/28小Subnet**

### 2. VNIC管理

```yaml
oci:
  vnicPreAllocationThreshold: 0.8  # 80%使用率时预创建
  maxIPsPerVNIC: 32                 # 根据实例形状调整
```

### 3. 监控和告警

```yaml
prometheus:
  enabled: true
  serviceMonitor:
    enabled: true

# 关键指标告警
# - cilium_oci_subnet_ips_used / cilium_oci_subnet_ips_total > 0.85
# - cilium_oci_vnic_creation_errors_total > 0
# - cilium_ipam_allocation_duration_seconds > 5
```

### 4. 高可用配置

```yaml
operator:
  replicas: 2  # 至少2个副本
  
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - topologyKey: kubernetes.io/hostname
```

### 5. 资源配额

```yaml
# 根据集群规模调整
operator:
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 1Gi

agent:
  resources:
    requests:
      cpu: 250m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi
```

### 6. 定期维护

- **每周**: 检查Subnet IP使用率
- **每月**: 审核VNIC数量和分布
- **每季度**: 更新Cilium版本（如有新版本）

---

## 快速参考卡片

### 部署命令速查

```bash
# 1. 准备values文件（修改VCN OCID等）
cat > cilium-oci-values.yaml <<EOF
...
EOF

# 2. 安装Cilium
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  -f cilium-oci-values.yaml

# 3. 验证
kubectl wait --for=condition=ready pod -l k8s-app=cilium -n kube-system --timeout=300s
cilium status

# 4. 创建测试Pod
kubectl create deployment test-nginx --image=nginx --replicas=3
kubectl get pods -o wide
```

### 故障排查速查

```bash
# Cilium状态
cilium status

# CiliumNode
kubectl get ciliumnode

# VNIC详情
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq

# Operator日志
kubectl logs -n kube-system deployment/cilium-operator --tail=100

# Agent日志
kubectl logs -n kube-system daemonset/cilium -c cilium-agent --tail=100

# IAM测试
oci iam region list --auth instance_principal
```

### Helm操作速查

```bash
# 查看当前配置
helm get values cilium -n kube-system

# 升级配置
helm upgrade cilium cilium/cilium -n kube-system --reuse-values --set key=value

# 回滚
helm rollback cilium -n kube-system

# 历史版本
helm history cilium -n kube-system
```

---

## 附录

### A. Subnet CIDR参考

| CIDR | 总IP | 可用IP | 适用场景 |
|------|------|--------|----------|
| /28 | 16 | 13 | ❌ 不推荐（太小） |
| /27 | 32 | 29 | ⚠️ 开发环境 |
| /26 | 64 | 61 | ⚠️ 小型集群 |
| /25 | 128 | 125 | ✅ 测试环境 |
| /24 | 256 | 251 | ✅ 生产环境（推荐） |
| /23 | 512 | 509 | ✅ 中型集群 |
| /22 | 1024 | 1021 | ✅ 大型集群 |
| /20 | 4096 | 4091 | ✅ 超大型集群 |

### B. 实例形状VNIC限制

| 实例形状 | 最大VNIC | 每VNIC最大IP |
|---------|---------|-------------|
| VM.Standard.E5.Flex | 2-8 | 32 |
| VM.Standard3.Flex | 2-8 | 32 |
| BM.Standard.E5.192 | 24 | 32 |
| VM.DenseIO.E5.Flex | 8 | 32 |

### C. 相关文档链接

- **完整部署手册**: `CILIUM_OCI_IPAM_DEPLOYMENT_MANUAL.md`
- **命令参考**: `CILIUM_OCI_IPAM_COMMAND_REFERENCE.md`
- **项目汇总**: `OCI_IPAM_MIGRATION_COMPLETE_SUMMARY.md`

---

**祝您部署顺利！** 🎉

如有问题，请参考故障排查章节或联系技术支持。

---

**文档版本**: 1.0  
**创建时间**: 2025年10月27日  
**维护者**: Dengwei (SEHUB)  
 
