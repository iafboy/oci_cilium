# Cilium OCI IPAM 部署手册

## 目录

1. [概述](#1-概述)
2. [环境准备](#2-环境准备)
3. [OCI IAM配置](#3-oci-iam配置)
4. [镜像准备](#4-镜像准备)
5. [Helm部署](#5-helm部署)
6. [部署验证](#6-部署验证)
7. [多VNIC配置](#7-多vnic配置)
8. [Hubble配置](#8-hubble配置)
9. [常见问题](#9-常见问题)

---

## 1. 概述

### 1.1 关于Cilium OCI IPAM

Cilium OCI IPAM是为Oracle Cloud Infrastructure (OCI)环境定制的IP地址管理解决方案，支持：

- **原生OCI集成**：直接调用OCI API管理IP
- **多VNIC支持**：自动管理多个VNIC扩展IP容量
- **高性能**：基于eBPF的数据平面
- **可观测性**：集成Hubble提供流量可视化

### 1.2 架构图

```
┌─────────────────────────────────────────────────────┐
│                  Kubernetes Cluster                  │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐ │
│  │ Cilium Agent │  │ Cilium Agent │  │  Cilium   │ │
│  │  (DaemonSet) │  │  (DaemonSet) │  │ Operator  │ │
│  │              │  │              │  │           │ │
│  │  OCI IPAM    │  │  OCI IPAM    │  │  OCI API  │ │
│  │  Allocator   │  │  Allocator   │  │  Manager  │ │
│  └──────┬───────┘  └──────┬───────┘  └─────┬─────┘ │
│         │                 │                 │       │
└─────────┼─────────────────┼─────────────────┼───────┘
          │                 │                 │
          └─────────────────┴─────────────────┘
                            │
                    ┌───────▼────────┐
                    │   OCI VCN API   │
                    │   - VNIC Mgmt   │
                    │   - IP Alloc    │
                    └─────────────────┘
```

### 1.3 系统要求

| 组件 | 要求 |
|------|------|
| **Kubernetes** | v1.21+ |
| **操作系统** | Ubuntu 20.04+ / Oracle Linux 8+ |
| **内核** | Linux 5.10+ (eBPF支持) |
| **CPU架构** | x86_64 / ARM64 |
| **网络插件** | 无需预装CNI |
| **OCI资源** | VCN, Subnets, IAM Policies |

---

## 2. 环境准备

### 2.1 OCI资源检查清单

在开始部署前，确保已准备：

- [ ] **VCN已创建** - 获取VCN OCID
- [ ] **Subnets已创建** - 至少1个Subnet用于Pod网络
- [ ] **Internet/NAT Gateway配置** - 根据需求选择
- [ ] **IAM Policy配置** - 见第3章
- [ ] **Compute Instances** - Kubernetes节点已部署
- [ ] **OCI CLI配置** - 管理节点可访问OCI API

### 2.2 VCN规划

#### 推荐的子网配置

```
VCN: 10.0.0.0/16
│
├── Subnet-1 (Management): 10.0.0.0/24
│   ├── Gateway: Internet Gateway (公共访问)
│   └── 用途: Kubernetes节点主网卡
│
├── Subnet-2 (Pod Network): 10.0.1.0/24
│   ├── Gateway: NAT Gateway (私有出站)
│   └── 用途: Pod IP地址池
│
└── Subnet-3 (Additional): 10.0.2.0/24
    ├── Gateway: NAT Gateway
    └── 用途: 多VNIC扩展 (可选)
```

#### 安全列表规则

**Ingress规则：**
```
Source          Protocol    Port        Description
10.0.0.0/16     ICMP        All         VCN内部通信
10.0.0.0/16     TCP         All         Kubernetes通信
0.0.0.0/0       TCP         6443        K8s API Server (如需外部访问)
0.0.0.0/0       TCP         30000-32767 NodePort服务 (可选)
```

**Egress规则：**
```
Destination     Protocol    Port        Description
0.0.0.0/0       All         All         允许所有出站流量
```

### 2.3 Kubernetes集群要求

```bash
# 检查集群版本
kubectl version

# 检查节点就绪状态
kubectl get nodes

# 检查是否有其他CNI插件（需要先删除）
kubectl get pods -n kube-system | grep -E 'calico|flannel|weave'

# 如有其他CNI，需要先清理
kubectl delete -f <previous-cni-manifest>.yaml
```

---

## 3. OCI IAM配置

### 3.1 认证方式选择

Cilium支持3种OCI认证方式：

| 认证方式 | 适用场景 | 配置复杂度 | 推荐度 |
|---------|---------|-----------|--------|
| **Instance Principal** | 生产环境（推荐） | 低 | ⭐⭐⭐⭐⭐ |
| **API Key** | 开发/测试 | 中 | ⭐⭐⭐ |
| **Security Token** | 临时访问 | 高 | ⭐⭐ |

### 3.2 配置Instance Principal（推荐）

#### 步骤1: 创建Dynamic Group

```bash
# 通过OCI Console或CLI创建Dynamic Group
oci iam dynamic-group create \
  --name cilium-instances \
  --description "Dynamic group for Cilium instances" \
  --matching-rule "Any {instance.compartment.id = '<compartment-ocid>'}"
```

#### 步骤2: 创建IAM Policy

创建Policy并附加到Compartment：

```hcl
# Policy: cilium-oci-ipam-policy

Allow dynamic-group cilium-instances to manage vnics in compartment <compartment-name>
Allow dynamic-group cilium-instances to use subnets in compartment <compartment-name>
Allow dynamic-group cilium-instances to use network-security-groups in compartment <compartment-name>
Allow dynamic-group cilium-instances to use private-ips in compartment <compartment-name>
Allow dynamic-group cilium-instances to inspect compartments in compartment <compartment-name>
Allow dynamic-group cilium-instances to inspect vcns in compartment <compartment-name>
Allow dynamic-group cilium-instances to read virtual-network-family in compartment <compartment-name>
```

#### 步骤3: 验证Instance Principal

```bash
# 在任意K8s节点上执行
oci iam region list --auth instance_principal

# 应该返回Region列表，表示认证成功
```

### 3.3 配置API Key（开发环境）

#### 步骤1: 生成API Key

```bash
mkdir -p ~/.oci
openssl genrsa -out ~/.oci/oci_api_key.pem 2048
chmod 600 ~/.oci/oci_api_key.pem
openssl rsa -pubout -in ~/.oci/oci_api_key.pem -out ~/.oci/oci_api_key_public.pem
```

#### 步骤2: 上传公钥到OCI

```bash
cat ~/.oci/oci_api_key_public.pem
# 复制输出，在OCI Console -> User Settings -> API Keys -> Add API Key
```

#### 步骤3: 创建配置文件

```bash
cat > ~/.oci/config <<EOF
[DEFAULT]
user=<user-ocid>
fingerprint=<key-fingerprint>
tenancy=<tenancy-ocid>
region=<region>
key_file=~/.oci/oci_api_key.pem
EOF
```

#### 步骤4: 创建Kubernetes Secret

```bash
kubectl create secret generic oci-credentials \
  --from-file=config=~/.oci/config \
  --from-file=key=~/.oci/oci_api_key.pem \
  -n kube-system
```

### 3.4 最小权限IAM Policy

如果只需基本IPAM功能（不创建VNIC），可使用最小权限：

```hcl
Allow group cilium-users to use private-ips in compartment <compartment-name>
Allow group cilium-users to inspect vnics in compartment <compartment-name>
Allow group cilium-users to inspect subnets in compartment <compartment-name>
Allow group cilium-users to inspect vcns in compartment <compartment-name>
```

---

## 4. 镜像准备

### 4.1 镜像列表

| 镜像 | 大小 | 说明 |
|------|------|------|
| `sin.ocir.io/sehubjapacprod/munger/agent:latest` | 589MB | Cilium Agent (含OCI IPAM) |
| `sin.ocir.io/sehubjapacprod/munger/operator:test-fix4` | 142MB | Cilium Operator (OCI IPAM) |
| `quay.io/cilium/hubble-relay:v1.15.2` | 45MB | Hubble Relay (可选) |
| `quay.io/cilium/hubble-ui:v0.12.1` | 32MB | Hubble UI (可选) |
| `quay.io/cilium/hubble-ui-backend:v0.12.1` | 28MB | Hubble UI Backend (可选) |

### 4.2 构建OCI IPAM镜像

#### 构建Cilium Operator（支持OCI IPAM）

```bash
# 进入项目目录
cd /home/ubuntu/xiaomi-cilium/cilium-official-fork-1022

# 构建并推送Operator镜像到OCI Registry
make build-container-operator-oci \
  DOCKER_DEV_ACCOUNT=sin.ocir.io/sehubjapacprod/munger \
  DOCKER_IMAGE_TAG=test-fix4 \
  DOCKER_FLAGS="--push"

# 等待构建完成，应该看到：
# Successfully built operator image
# Successfully pushed to sin.ocir.io/sehubjapacprod/munger/operator:test-fix4
```

#### 构建Cilium Agent

```bash
# 构建并推送Agent镜像（如需自定义）
make build-container-agent \
  DOCKER_DEV_ACCOUNT=sin.ocir.io/sehubjapacprod/munger \
  DOCKER_IMAGE_TAG=latest \
  DOCKER_FLAGS="--push"
```

**注意事项：**
- 构建过程需要15-20分钟
- 确保Docker已登录OCI Registry
- 构建完成后验证镜像是否成功推送

### 4.3 使用OCI Registry（推荐）

#### 登录OCI Registry

```bash
# 获取Auth Token (OCI Console -> User Settings -> Auth Tokens)
docker login sin.ocir.io -u '<tenancy-namespace>/<username>' -p '<auth-token>'

# 验证登录
docker info | grep -A 3 "Registry Mirrors"
```

#### 从私有Registry拉取镜像

```bash
# 在每个节点上执行（或通过imagePullSecrets自动拉取）
docker pull sin.ocir.io/sehubjapacprod/munger/agent:latest
docker pull sin.ocir.io/sehubjapacprod/munger/operator:test-fix4

# 验证镜像
docker images | grep munger
```

#### 创建imagePullSecrets

```bash
kubectl create secret docker-registry ocir-secret \
  --docker-server=sin.ocir.io \
  --docker-username='<tenancy-namespace>/<username>' \
  --docker-password='<auth-token>' \
  -n kube-system

# 验证Secret创建
kubectl get secret ocir-secret -n kube-system
```

### 4.4 离线镜像导入（无Internet访问）

适用于无法访问外网的环境：

```bash
# === 步骤1: 在有网环境导出镜像 ===
docker save sin.ocir.io/sehubjapacprod/munger/agent:latest -o cilium-agent.tar
docker save sin.ocir.io/sehubjapacprod/munger/operator:test-fix4 -o cilium-operator.tar

# === 步骤2: 传输到各节点 ===
scp cilium-agent.tar ubuntu@<node-ip>:/tmp/
scp cilium-operator.tar ubuntu@<node-ip>:/tmp/

# === 步骤3: 在各节点导入 ===
ssh ubuntu@<node-ip>
docker load -i /tmp/cilium-agent.tar
docker load -i /tmp/cilium-operator.tar

# 验证镜像已导入
docker images | grep munger
```

---

## 5. Helm部署

### 5.1 获取Helm Chart

```bash
# 克隆Cilium仓库（OCI IPAM分支）
git clone https://github.com/<your-org>/cilium.git -b feature/oci-fork
cd cilium

# 或直接使用本地Chart
cd /home/ubuntu/xiaomi-cilium/dw-bak-code
```

### 5.2 创建values.yaml配置文件

创建 `oci-ipam-values.yaml`：

```yaml
# =====================================================
# Cilium OCI IPAM 配置文件
# =====================================================

# --- 基础配置 ---
cluster:
  name: cilium-oci-cluster
  id: 1

# --- 镜像配置 ---
image:
  repository: sin.ocir.io/sehubjapacprod/munger/agent
  tag: latest
  pullPolicy: IfNotPresent

imagePullSecrets:
  - name: ocir-secret

operator:
  image:
    repository: sin.ocir.io/sehubjapacprod/munger/operator
    tag: test-fix4
    pullPolicy: IfNotPresent
  replicas: 1

# --- IPAM配置 ---
ipam:
  mode: oci
  operator:
    clusterPoolIPv4PodCIDRList:
      - 10.0.0.0/16  # 根据实际VCN CIDR调整

# --- OCI IPAM配置 ---
oci:
  enabled: true
  vcnID: "ocid1.vcn.oc1.ap-singapore-1.xxxxx"  # 替换为实际VCN OCID
  subnetOCID: "ocid1.subnet.oc1.ap-singapore-1.xxxxx"  # 主Subnet OCID
  useInstancePrincipal: true  # 使用Instance Principal认证
  
  # 如果使用API Key，配置以下项：
  # useInstancePrincipal: false
  # configMapName: "oci-config"
  # secretName: "oci-credentials"

# --- 网络配置 ---
tunnel: disabled  # OCI使用native routing
autoDirectNodeRoutes: true
ipv4NativeRoutingCIDR: 10.0.0.0/16
endpointRoutes:
  enabled: true

# --- Hubble配置 ---
hubble:
  enabled: true
  listenAddress: ":4244"
  
  tls:
    enabled: false  # 简化配置，生产环境建议启用
  
  metrics:
    enabled:
      - dns
      - drop
      - tcp
      - flow
      - port-distribution
      - icmp
      - http
  
  relay:
    enabled: true
    replicas: 1
  
  ui:
    enabled: true
    replicas: 1

# --- 资源限制 ---
resources:
  limits:
    cpu: 4000m
    memory: 4Gi
  requests:
    cpu: 1000m
    memory: 1Gi

operator:
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 100m
      memory: 128Mi

# --- 其他配置 ---
kubeProxyReplacement: "strict"  # 使用Cilium替代kube-proxy
k8sServiceHost: "<k8s-api-server-ip>"
k8sServicePort: "6443"

# === 调试配置（可选） ===
debug:
  enabled: false
  # verbose: "flow"  # 需要详细日志时启用
```

### 5.3 执行Helm安装

```bash
# === 方式1: 使用Helm安装（推荐） ===
helm install cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --values oci-ipam-values.yaml

# === 方式2: 命令行参数安装 ===
helm install cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --set ipam.mode=oci \
  --set oci.enabled=true \
  --set oci.vcnID="ocid1.vcn.oc1.ap-singapore-1.xxxxx" \
  --set oci.subnetOCID="ocid1.subnet.oc1.ap-singapore-1.xxxxx" \
  --set oci.useInstancePrincipal=true \
  --set image.repository=sin.ocir.io/sehubjapacprod/munger/agent \
  --set image.tag=latest \
  --set operator.image.repository=sin.ocir.io/sehubjapacprod/munger/operator \
  --set operator.image.tag=test-fix4 \
  --set tunnel=disabled \
  --set autoDirectNodeRoutes=true
```

### 5.4 验证部署状态

```bash
# 检查Helm Release
helm list -n kube-system

# 检查Pod状态
kubectl get pods -n kube-system -l k8s-app=cilium

# 应该看到：
# NAME                               READY   STATUS    RESTARTS   AGE
# cilium-operator-xxxxxxxxxx-xxxxx   1/1     Running   0          2m
# cilium-xxxxx                       1/1     Running   0          2m
# cilium-xxxxx                       1/1     Running   0          2m
# cilium-xxxxx                       1/1     Running   0          2m
```

### 5.5 升级现有部署

```bash
# 修改values.yaml后升级
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --values oci-ipam-values.yaml \
  --reuse-values  # 保留未修改的配置

# 查看升级历史
helm history cilium -n kube-system

# 回滚到上一版本（如需）
helm rollback cilium <revision> -n kube-system
```

---

## 6. 部署验证

### 6.1 检查Cilium状态

```bash
# === 安装Cilium CLI（推荐） ===
CILIUM_CLI_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)
CLI_ARCH=amd64
curl -L --fail --remote-name-all https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${CLI_ARCH}.tar.gz
sudo tar xzvfC cilium-linux-${CLI_ARCH}.tar.gz /usr/local/bin
rm cilium-linux-${CLI_ARCH}.tar.gz

# 运行Cilium状态检查
cilium status --wait

# 期望输出：
#     /¯¯\
#  /¯¯\__/¯¯\    Cilium:             OK
#  \__/¯¯\__/    Operator:           OK
#  /¯¯\__/¯¯\    Hubble Relay:       OK
#  \__/¯¯\__/    ClusterMesh:        disabled
#     \__/
```

### 6.2 验证OCI IPAM配置

```bash
# 检查CiliumNode CRD
kubectl get ciliumnodes

# 查看详细信息
kubectl get ciliumnode <node-name> -o yaml

# 验证OCI配置是否正确：
# spec:
#   oci:
#     vcnID: "ocid1.vcn.oc1.ap-singapore-1.xxxxx"
#     instanceID: "ocid1.instance.oc1.ap-singapore-1.xxxxx"
```

### 6.3 验证Pod IP分配

```bash
# 创建测试Pod
kubectl run test-nginx --image=nginx

# 等待Pod运行
kubectl wait --for=condition=Ready pod/test-nginx --timeout=60s

# 检查Pod IP
kubectl get pod test-nginx -o wide

# 验证IP在VCN CIDR范围内
# 例如: 10.0.0.103/24
```

### 6.4 网络连通性测试

```bash
# === 测试1: Pod ↔ Pod ===
kubectl run test-client --image=busybox --rm -it -- sh
# 在Pod内执行
ping <test-nginx-pod-ip>
# 应该成功ping通

# === 测试2: Pod → Service ===
kubectl expose pod test-nginx --port=80 --name=test-svc
kubectl run test-client --image=busybox --rm -it -- sh
# 在Pod内执行
wget -O- http://test-svc
# 应该返回nginx默认页面

# === 测试3: Pod → 外部DNS ===
kubectl run test-client --image=busybox --rm -it -- sh
# 在Pod内执行
nslookup google.com
# 应该解析成功

# === 测试4: Node → Pod ===
# 在任意节点执行
curl http://<pod-ip>
# 应该返回nginx页面
```

### 6.5 检查日志

```bash
# Cilium Agent日志
kubectl logs -n kube-system -l k8s-app=cilium --tail=100

# 搜索OCI相关日志
kubectl logs -n kube-system -l k8s-app=cilium | grep -i oci

# Operator日志
kubectl logs -n kube-system deployment/cilium-operator

# 搜索错误
kubectl logs -n kube-system deployment/cilium-operator | grep -i error
```

---

## 7. 多VNIC配置

多VNIC配置用于扩展单节点的IP容量（每个VNIC默认支持32个Private IP）。

### 7.1 使用场景

- **单节点Pod密度 > 32**：需要更多IP地址
- **网络隔离需求**：不同Subnet的Pod分离
- **高性能需求**：分散网络流量到多个VNIC

### 7.2 创建额外VNIC

#### 通过OCI Console

1. 导航到 **Compute → Instances → <节点实例>**
2. 点击 **Attached VNICs**
3. 点击 **Create VNIC**
4. 配置：
   - VNIC名称: `cilium-vnic-2`
   - Subnet: 选择Pod网络Subnet
   - Private IP: 自动分配或指定
   - Skip source/destination check: ✅ **勾选**
5. 点击 **Create VNIC**

#### 通过OCI CLI

```bash
# 获取实例OCID
INSTANCE_ID="ocid1.instance.oc1.ap-singapore-1.xxxxx"
SUBNET_ID="ocid1.subnet.oc1.ap-singapore-1.xxxxx"

# 创建VNIC
oci compute instance attach-vnic \
  --instance-id $INSTANCE_ID \
  --subnet-id $SUBNET_ID \
  --display-name cilium-vnic-2 \
  --skip-source-dest-check true \
  --wait-for-state ATTACHED

# 获取新VNIC的OCID
oci compute vnic-attachment list \
  --instance-id $INSTANCE_ID \
  --query 'data[?display-name==`cilium-vnic-2`].[vnic-id]' \
  --output table
```

### 7.3 配置节点网络接口

```bash
# SSH到节点
ssh ubuntu@<node-ip>

# 查看网络接口
ip addr show

# 应该看到新接口（例如：enp1s0）
# 但可能没有IP地址

# 获取VNIC的Private IP
VNIC_ID="ocid1.vnic.oc1.ap-singapore-1.xxxxx"
PRIVATE_IP=$(oci network vnic get --vnic-id $VNIC_ID --query 'data."private-ip"' --raw-output)

# 配置接口IP（临时）
sudo ip addr add ${PRIVATE_IP}/24 dev enp1s0
sudo ip link set enp1s0 up

# 添加路由（如需要）
sudo ip route add 10.0.0.0/16 dev enp1s0
```

#### 持久化配置（Ubuntu）

```bash
sudo tee /etc/netplan/60-vnic2.yaml <<EOF
network:
  version: 2
  ethernets:
    enp1s0:
      dhcp4: true
      dhcp4-overrides:
        use-routes: false
      routes:
        - to: 10.0.0.0/16
          via: <vnic2-gateway>
          table: 210
      routing-policy:
        - from: <vnic2-private-ip>
          table: 210
EOF

sudo netplan apply
```

### 7.4 验证多VNIC功能

```bash
# 创建大量Pod触发多VNIC使用
for i in {1..40}; do
  kubectl run test-multi-vnic-$i \
    --image=busybox \
    --overrides='{"spec":{"nodeSelector":{"kubernetes.io/hostname":"<node-with-multi-vnic>"}}}' \
    -- sleep 3600
done

# 等待所有Pod运行
kubectl wait --for=condition=Ready pod -l run --timeout=180s

# 检查Pod IP分配
kubectl get pods -o wide | grep test-multi-vnic

# 应该看到来自不同Subnet/VNIC的IP：
# test-multi-vnic-1   10.0.0.105  (VNIC-1)
# test-multi-vnic-2   10.0.0.106  (VNIC-1)
# ...
# test-multi-vnic-33  10.0.1.10   (VNIC-2)  ← 新VNIC的IP
# test-multi-vnic-34  10.0.1.11   (VNIC-2)
```

### 7.5 多VNIC限制

- **每实例最多16个VNIC**（取决于Shape）
- **每VNIC最多32个Secondary Private IP**
- **总IP容量 = VNIC数量 × 32**
- 例如：2个VNIC = 64个Pod IP

查看实例支持的VNIC数量：
```bash
oci compute shape list --compartment-id <compartment-ocid> \
  | grep -A 5 "VM.Standard.E4.Flex"
```

---

## 8. Hubble配置

Hubble提供强大的可观测性功能，建议在生产环境启用。

### 8.1 启用Hubble

如果初始安装时未启用，可通过Helm升级：

```bash
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --set hubble.enabled=true \
  --set hubble.listenAddress=":4244" \
  --set hubble.tls.enabled=false \
  --set hubble.metrics.enabled="{dns,drop,tcp,flow,port-distribution,icmp,http}" \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true
```

### 8.2 验证Hubble组件

```bash
# 检查Hubble Pods
kubectl get pods -n kube-system | grep hubble

# 期望输出：
# hubble-relay-xxxxxxxxxx-xxxxx    1/1     Running
# hubble-ui-xxxxxxxxxx-xxxxx       2/2     Running

# 检查Hubble Relay日志
kubectl logs -n kube-system deployment/hubble-relay

# 应该看到：
# level=info msg=Connected address="10.0.0.141:4244" peer=cilium-w1
# level=info msg=Connected address="10.0.0.132:4244" peer=cilium-w2
# level=info msg=Connected address="10.0.0.234:4244" peer=cilium-m
```

### 8.3 访问Hubble UI

#### 方式1: Port Forward（快速测试）

```bash
# 本地访问
kubectl port-forward -n kube-system svc/hubble-ui 12000:80

# 远程访问（绑定所有接口）
kubectl port-forward -n kube-system svc/hubble-ui 12000:80 --address 0.0.0.0

# 浏览器访问
open http://localhost:12000
```

#### 方式2: NodePort（持久访问）

```bash
# 修改Service为NodePort
kubectl patch svc -n kube-system hubble-ui -p '{"spec":{"type":"NodePort"}}'

# 获取NodePort
kubectl get svc -n kube-system hubble-ui

# 输出示例：
# NAME        TYPE       CLUSTER-IP      EXTERNAL-IP   PORT(S)        AGE
# hubble-ui   NodePort   10.96.123.45    <none>        80:31234/TCP   5m

# 访问任意节点的31234端口
open http://<node-ip>:31234
```

#### 方式3: Ingress（生产环境）

```yaml
# hubble-ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hubble-ui
  namespace: kube-system
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
  - host: hubble.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: hubble-ui
            port:
              number: 80
```

```bash
kubectl apply -f hubble-ingress.yaml
```

### 8.4 使用Hubble CLI

```bash
# 安装Hubble CLI
HUBBLE_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/hubble/master/stable.txt)
HUBBLE_ARCH=amd64
curl -L --fail --remote-name-all https://github.com/cilium/hubble/releases/download/$HUBBLE_VERSION/hubble-linux-${HUBBLE_ARCH}.tar.gz
sudo tar xzvfC hubble-linux-${HUBBLE_ARCH}.tar.gz /usr/local/bin
rm hubble-linux-${HUBBLE_ARCH}.tar.gz

# 配置Hubble CLI连接
cilium hubble port-forward &

# 查看实时流量
hubble observe

# 查看特定Pod的流量
hubble observe --pod <pod-name>

# 查看被drop的包
hubble observe --verdict DROPPED

# 查看HTTP流量
hubble observe --protocol http
```

---

## 9. 常见问题

### 9.1 部署问题

#### Q1: Agent Pod一直Pending

**症状：**
```
cilium-xxxxx   0/1     Pending   0          5m
```

**原因 & 解决：**
```bash
# 检查事件
kubectl describe pod -n kube-system cilium-xxxxx

# 常见原因：
# 1. 镜像拉取失败
#    → 检查imagePullSecrets配置
#    → 验证Registry访问权限

# 2. 节点资源不足
#    → kubectl describe node
#    → 增加节点资源或减少资源requests

# 3. 污点/容忍度问题
#    → 检查节点Taints
#    → 添加tolerations到DaemonSet
```

#### Q2: Operator无法连接OCI API

**症状：**
```
level=error msg="Failed to get VCN" error="401 Unauthorized"
```

**原因 & 解决：**
```bash
# 1. 检查Instance Principal配置
oci iam region list --auth instance_principal

# 2. 检查IAM Policy
#    确保Dynamic Group包含该实例
#    确保Policy授予了必要权限

# 3. 检查VCN OCID是否正确
kubectl get configmap -n kube-system cilium-config -o yaml | grep oci-vcn-id

# 4. 重启Operator
kubectl rollout restart deployment/cilium-operator -n kube-system
```

#### Q3: Pod无法获取IP

**症状：**
```
test-pod   0/1     ContainerCreating   0          2m
```

**诊断：**
```bash
# 检查事件
kubectl describe pod test-pod

# 查看Cilium日志
kubectl logs -n kube-system -l k8s-app=cilium | grep -i "ip allocation"

# 检查CiliumNode状态
kubectl get ciliumnode -o yaml

# 常见原因：
# 1. Subnet IP耗尽
#    → 添加多VNIC
#    → 使用更大的Subnet

# 2. OCI API限流
#    → 等待重试
#    → 联系OCI support增加配额

# 3. VNIC权限问题
#    → 检查IAM Policy中的private-ips权限
```

### 9.2 网络问题

#### Q4: Pod无法访问Internet

**症状：**
```bash
kubectl exec test-pod -- ping 8.8.8.8
# timeout
```

**解决步骤：**
```bash
# 1. 检查Subnet路由表
#    确保有到Internet Gateway或NAT Gateway的路由

# 2. 检查安全列表/网络安全组
#    确保允许出站流量

# 3. 检查VNIC Source/Dest Check
oci network vnic get --vnic-id <vnic-id> | grep skip-source-dest-check
# 应该为 true

# 如果为false，启用：
oci network vnic update --vnic-id <vnic-id> --skip-source-dest-check true

# 4. 检查节点路由
ssh <node-ip>
ip route show
# 确保有默认路由
```

#### Q5: Pod之间无法通信

**诊断：**
```bash
# 1. 检查Cilium状态
cilium status

# 2. 检查Cilium Network Policy
kubectl get cnp --all-namespaces

# 3. 使用Hubble观察流量
hubble observe --pod <source-pod>

# 4. 检查节点间路由
#    OCI模式下需要VCN内路由配置正确
```

### 9.3 Hubble问题

#### Q6: Hubble Relay CrashLoopBackOff

**症状：**
```
hubble-relay-xxxxx   0/1     CrashLoopBackOff   5          3m
```

**解决：**
```bash
# 1. 检查TLS配置
#    常见问题：TLS证书问题
helm upgrade cilium ./install/kubernetes/cilium \
  --reuse-values \
  --set hubble.tls.enabled=false

# 2. 检查端口配置
kubectl get configmap -n kube-system cilium-config -o yaml | grep hubble-listen-address
# 应该是 :4244

# 3. 查看详细日志
kubectl logs -n kube-system deployment/hubble-relay
```

#### Q7: Hubble UI无法显示数据

**检查：**
```bash
# 1. 确认Relay正常连接到Agent
kubectl logs -n kube-system deployment/hubble-relay | grep Connected

# 2. 检查metrics配置
helm get values cilium -n kube-system | grep -A 10 metrics

# 3. 测试Relay连接
kubectl port-forward -n kube-system svc/hubble-relay 4245:80
hubble observe --server localhost:4245
```

### 9.4 性能问题

#### Q8: Pod创建缓慢

**优化：**
```bash
# 1. 增加Operator资源
helm upgrade cilium ./install/kubernetes/cilium \
  --reuse-values \
  --set operator.resources.requests.cpu=500m \
  --set operator.resources.requests.memory=512Mi

# 2. 启用IP预分配
#    修改CiliumNode spec:
#      ipam:
#        pre-allocate: 16

# 3. 减少API调用频率
#    配置更大的IP pool
```

### 9.5 升级问题

#### Q9: 从v1.13升级到v1.15后Pod无IP

**回滚步骤：**
```bash
# 1. 查看Helm历史
helm history cilium -n kube-system

# 2. 回滚到上一版本
helm rollback cilium <revision> -n kube-system

# 3. 检查CRD兼容性
kubectl get crd ciliumnodes.cilium.io -o yaml

# 4. 如需重新升级：
#    - 确保OCI配置正确
#    - 使用 --force 重建Pods
helm upgrade cilium ./install/kubernetes/cilium \
  --force \
  --values oci-ipam-values.yaml
```

### 9.6 Operator镜像和权限问题

#### Q10: Operator Pod启动失败或OCI API调用权限错误

**症状：**
```bash
# Operator日志显示权限错误
kubectl logs -n kube-system deployment/cilium-operator
# Error: operator.cloud is not defined in the build

# 或者
# Error: 401 Unauthorized when calling OCI API
```

**原因：**
Operator镜像构建时未正确包含OCI IPAM provider，导致运行时缺少OCI API访问能力。

**解决方案：**

1. **重新构建Operator镜像（包含OCI IPAM支持）**
```bash
cd /home/ubuntu/xiaomi-cilium/cilium-official-fork-1022

# 使用正确的构建目标
make build-container-operator-oci \
  DOCKER_DEV_ACCOUNT=sin.ocir.io/sehubjapacprod/munger \
  DOCKER_IMAGE_TAG=test-fix4 \
  DOCKER_FLAGS="--push"

# 验证镜像构建成功
docker images | grep operator
```

2. **检查Makefile配置**
```bash
# 确保 install/kubernetes/Makefile.values 中定义了 operator.cloud
grep "operator.cloud" ./install/kubernetes/Makefile.values

# 应该看到类似：
# operator.cloud ?= generic
```

3. **更新Helm部署使用新镜像**
```bash
# 删除旧的部署
kubectl delete deployment cilium-operator -n kube-system

# 使用新镜像重新安装
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --set operator.image.tag=test-fix4 \
  --force
```

4. **验证Operator运行正常**
```bash
# 检查Pod状态
kubectl get pods -n kube-system -l name=cilium-operator

# 查看日志确认OCI API访问正常
kubectl logs -n kube-system deployment/cilium-operator | grep -i oci

# 应该看到类似：
# level=info msg="OCI IPAM provider initialized"
# level=info msg="Successfully connected to OCI API"
```

**预防措施：**
- 始终使用 `make build-container-operator-oci` 而不是通用的 operator 目标
- 在CI/CD流程中添加构建验证步骤
- 部署前验证镜像是否包含OCI provider：
  ```bash
  docker run --rm sin.ocir.io/sehubjapacprod/munger/operator:test-fix4 \
    cilium-operator-oci --version
  ```

---

## 10. 附录

### 10.1 完整命令速查

```bash
# === 安装 ===
helm install cilium ./install/kubernetes/cilium -n kube-system -f oci-ipam-values.yaml

# === 验证 ===
cilium status --wait
kubectl get pods -n kube-system -l k8s-app=cilium
kubectl get ciliumnodes

# === 测试 ===
kubectl run test-nginx --image=nginx
kubectl expose pod test-nginx --port=80
kubectl run test-client --image=busybox --rm -it -- wget -O- http://test-nginx

# === 观察 ===
hubble observe
kubectl logs -n kube-system -l k8s-app=cilium --tail=100

# === 故障排查 ===
cilium status --wait
kubectl get events -n kube-system --sort-by='.lastTimestamp'
kubectl describe ciliumnode <node-name>

# === 清理 ===
helm uninstall cilium -n kube-system
kubectl delete crd ciliumnodes.cilium.io
```

### 10.2 参考资源

- **Cilium官方文档**: https://docs.cilium.io
- **OCI文档**: https://docs.oracle.com/en-us/iaas/
- **GitHub仓库**: https://github.com/cilium/cilium
- **社区支持**: https://cilium.io/slack

### 10.3 技术支持

遇到问题请联系：
- **邮箱**: CE&&SEHUB 


---

**文档版本：** 1.0  
**最后更新：** 2025年10月21日  
**维护人：** Dengwei
