# Cilium OCI IPAM 命令快速参考手册

## 目录

1. [部署命令](#1-部署命令)
2. [验证命令](#2-验证命令)
3. [网络测试命令](#3-网络测试命令)
4. [故障排查命令](#4-故障排查命令)
5. [OCI资源管理](#5-oci资源管理)
6. [多VNIC操作](#6-多vnic操作)
7. [Hubble操作](#7-hubble操作)
8. [性能监控](#8-性能监控)
9. [日常运维](#9-日常运维)
10. [一键脚本](#10-一键脚本)

---

## 1. 部署命令

### 1.1 Helm安装

```bash
# === 基础安装 ===
helm install cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --set ipam.mode=oci \
  --set oci.enabled=true \
  --set oci.vcnID="<vcn-ocid>" \
  --set oci.subnetOCID="<subnet-ocid>" \
  --set oci.useInstancePrincipal=true \
  --set image.repository=sin.ocir.io/sehubjapacprod/munger/agent \
  --set image.tag=latest \
  --set operator.image.repository=sin.ocir.io/sehubjapacprod/munger/operator \
  --set operator.image.tag=test-fix4 \
  --set tunnel=disabled \
  --set autoDirectNodeRoutes=true

# === 使用values.yaml安装 ===
helm install cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --values oci-ipam-values.yaml

# === 查看安装状态 ===
helm status cilium -n kube-system

# === 查看安装值 ===
helm get values cilium -n kube-system

# === 查看完整配置（包括默认值） ===
helm get values cilium -n kube-system --all
```

### 1.2 Helm升级

```bash
# === 升级配置 ===
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --set <key>=<value>

# === 示例：启用Hubble ===
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --set hubble.enabled=true \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true

# === 强制重建Pods ===
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --force

# === 查看升级历史 ===
helm history cilium -n kube-system

# === 回滚到上一版本 ===
helm rollback cilium -n kube-system

# === 回滚到指定版本 ===
helm rollback cilium <revision> -n kube-system
```

### 1.3 镜像管理

```bash
# === 登录OCI Registry ===
docker login sin.ocir.io \
  -u '<tenancy-namespace>/<username>' \
  -p '<auth-token>'

# === 拉取镜像 ===
docker pull sin.ocir.io/sehubjapacprod/munger/agent:latest
docker pull sin.ocir.io/sehubjapacprod/munger/operator:test-fix4

# === 查看本地镜像 ===
docker images | grep munger

# === 构建自定义镜像 ===
# 构建Operator镜像（支持OCI IPAM）
cd /path/to/cilium-source
make build-container-operator-oci \
  DOCKER_DEV_ACCOUNT=sin.ocir.io/sehubjapacprod/munger \
  DOCKER_IMAGE_TAG=test-fix4 \
  DOCKER_FLAGS="--push"

# 构建Agent镜像
make build-container-agent \
  DOCKER_DEV_ACCOUNT=sin.ocir.io/sehubjapacprod/munger \
  DOCKER_IMAGE_TAG=latest \
  DOCKER_FLAGS="--push"

# === 验证镜像内容 ===
# 验证Operator包含OCI IPAM支持
docker run --rm sin.ocir.io/sehubjapacprod/munger/operator:test-fix4 \
  cilium-operator-oci --version

# 列出二进制文件
docker run --rm sin.ocir.io/sehubjapacprod/munger/operator:test-fix4 \
  ls -la /usr/bin/ | grep cilium

# === 导出镜像（离线环境） ===
docker save sin.ocir.io/sehubjapacprod/munger/agent:latest \
  -o cilium-agent.tar
docker save sin.ocir.io/sehubjapacprod/munger/operator:test-fix4 \
  -o cilium-operator.tar

# === 导入镜像 ===
docker load -i cilium-agent.tar
docker load -i cilium-operator.tar

# === 创建imagePullSecret ===
kubectl create secret docker-registry ocir-secret \
  --docker-server=sin.ocir.io \
  --docker-username='<tenancy-namespace>/<username>' \
  --docker-password='<auth-token>' \
  -n kube-system

# === 查看Secret ===
kubectl get secret ocir-secret -n kube-system -o yaml

# === 删除并重建Secret ===
kubectl delete secret ocir-secret -n kube-system
kubectl create secret docker-registry ocir-secret \
  --docker-server=sin.ocir.io \
  --docker-username='<new-tenancy>/<new-user>' \
  --docker-password='<new-token>' \
  -n kube-system
```

---

## 2. 验证命令

### 2.1 Cilium状态检查

```bash
# === 安装Cilium CLI ===
CILIUM_CLI_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)
CLI_ARCH=amd64
curl -L --fail --remote-name-all \
  https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${CLI_ARCH}.tar.gz
sudo tar xzvfC cilium-linux-${CLI_ARCH}.tar.gz /usr/local/bin
rm cilium-linux-${CLI_ARCH}.tar.gz

# === 检查Cilium状态 ===
cilium status

# === 等待Cilium就绪 ===
cilium status --wait

# === 详细状态 ===
cilium status --verbose

# === 检查连接性 ===
cilium connectivity test
```

### 2.2 Pod状态检查

```bash
# === 查看所有Cilium Pods ===
kubectl get pods -n kube-system -l k8s-app=cilium -o wide

# === 查看Operator ===
kubectl get pods -n kube-system -l name=cilium-operator -o wide

# === 查看Hubble ===
kubectl get pods -n kube-system | grep hubble

# === 等待Pod就绪 ===
kubectl wait --for=condition=Ready pod -l k8s-app=cilium \
  -n kube-system --timeout=120s

# === 查看Pod详情 ===
kubectl describe pod -n kube-system <cilium-pod-name>

# === 查看Pod事件 ===
kubectl get events -n kube-system --field-selector involvedObject.name=<pod-name>
```

### 2.3 配置验证

```bash
# === 查看ConfigMap ===
kubectl get configmap -n kube-system cilium-config -o yaml

# === 查看OCI配置 ===
kubectl get configmap -n kube-system cilium-config -o yaml | grep oci

# === 查看IPAM配置 ===
kubectl get configmap -n kube-system cilium-config -o yaml | grep ipam

# === 查看Hubble配置 ===
kubectl get configmap -n kube-system cilium-config -o yaml | grep hubble

# === 查看CiliumNode CRD ===
kubectl get ciliumnodes

# === 查看特定节点的CiliumNode ===
kubectl get ciliumnode <node-name> -o yaml

# === 查看所有CiliumNode的IPAM状态 ===
kubectl get ciliumnodes -o custom-columns=\
NAME:.metadata.name,\
USED:.status.ipam.used,\
AVAILABLE:.status.ipam.available
```

### 2.4 服务检查

```bash
# === 查看Cilium相关Services ===
kubectl get svc -n kube-system | grep cilium

# === 查看Hubble Services ===
kubectl get svc -n kube-system | grep hubble

# === 查看Service详情 ===
kubectl describe svc -n kube-system cilium-operator

# === 查看Service Endpoints ===
kubectl get endpoints -n kube-system cilium-operator
```

---

## 3. 网络测试命令

### 3.1 基础连通性测试

```bash
# === 创建测试Pods ===
kubectl run test-client --image=busybox --restart=Never -- sleep 3600
kubectl run test-server --image=nginx --restart=Never

# === 等待Pods就绪 ===
kubectl wait --for=condition=Ready pod/test-client --timeout=60s
kubectl wait --for=condition=Ready pod/test-server --timeout=60s

# === 获取Pod IP ===
CLIENT_IP=$(kubectl get pod test-client -o jsonpath='{.status.podIP}')
SERVER_IP=$(kubectl get pod test-server -o jsonpath='{.status.podIP}')
echo "Client IP: $CLIENT_IP"
echo "Server IP: $SERVER_IP"

# === 测试1: Pod → Pod (ICMP) ===
kubectl exec test-client -- ping -c 3 $SERVER_IP

# === 测试2: Pod → Pod (TCP) ===
kubectl exec test-client -- wget -O- http://$SERVER_IP --timeout=5

# === 测试3: Pod → Service (ClusterIP) ===
kubectl expose pod test-server --port=80 --name=test-svc
kubectl exec test-client -- wget -O- http://test-svc --timeout=5

# === 测试4: Pod → Service (DNS) ===
kubectl exec test-client -- nslookup test-svc
kubectl exec test-client -- nslookup kubernetes.default

# === 测试5: Pod → Internet ===
kubectl exec test-client -- ping -c 3 8.8.8.8
kubectl exec test-client -- wget -O- http://www.google.com --timeout=5

# === 清理测试资源 ===
kubectl delete pod test-client test-server
kubectl delete svc test-svc
```

### 3.2 批量测试

```bash
# === 创建多个测试Pods ===
for i in {1..5}; do
  kubectl run test-pod-$i --image=nginx --labels="app=test"
done

# === 等待所有Pods就绪 ===
kubectl wait --for=condition=Ready pod -l app=test --timeout=120s

# === 查看所有测试Pod的IP ===
kubectl get pods -l app=test -o wide

# === 测试Pod间通信矩阵 ===
for src in $(kubectl get pods -l app=test -o jsonpath='{.items[*].metadata.name}'); do
  echo "Testing from $src:"
  for dst in $(kubectl get pods -l app=test -o jsonpath='{.items[*].status.podIP}'); do
    kubectl exec $src -- ping -c 1 -W 1 $dst > /dev/null 2>&1
    if [ $? -eq 0 ]; then
      echo "  ✓ $dst reachable"
    else
      echo "  ✗ $dst unreachable"
    fi
  done
done

# === 清理 ===
kubectl delete pods -l app=test
```

### 3.3 性能测试

```bash
# === 创建iperf测试 ===
kubectl run iperf-server --image=networkstatic/iperf3 -- iperf3 -s
kubectl run iperf-client --image=networkstatic/iperf3 -- sleep 3600

# === 等待就绪 ===
kubectl wait --for=condition=Ready pod/iperf-server --timeout=60s
kubectl wait --for=condition=Ready pod/iperf-client --timeout=60s

# === 获取Server IP ===
SERVER_IP=$(kubectl get pod iperf-server -o jsonpath='{.status.podIP}')

# === 运行TCP带宽测试 ===
kubectl exec iperf-client -- iperf3 -c $SERVER_IP -t 10

# === 运行UDP带宽测试 ===
kubectl exec iperf-client -- iperf3 -c $SERVER_IP -u -b 1G -t 10

# === 运行并行连接测试 ===
kubectl exec iperf-client -- iperf3 -c $SERVER_IP -P 10 -t 10

# === 清理 ===
kubectl delete pod iperf-server iperf-client
```

### 3.4 延迟测试

```bash
# === 创建测试Pods ===
kubectl run latency-test-1 --image=busybox -- sleep 3600
kubectl run latency-test-2 --image=busybox -- sleep 3600

# === 测试延迟 ===
POD2_IP=$(kubectl get pod latency-test-2 -o jsonpath='{.status.podIP}')
kubectl exec latency-test-1 -- ping -c 100 $POD2_IP | tail -1

# === 测试到节点的延迟 ===
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
kubectl exec latency-test-1 -- ping -c 100 $NODE_IP | tail -1

# === 清理 ===
kubectl delete pod latency-test-1 latency-test-2
```

---

## 4. 故障排查命令

### 4.1 日志查看

```bash
# === Cilium Agent日志 ===
kubectl logs -n kube-system -l k8s-app=cilium --tail=100

# === 查看所有Agent日志 ===
kubectl logs -n kube-system -l k8s-app=cilium --all-containers=true

# === 跟踪实时日志 ===
kubectl logs -n kube-system -l k8s-app=cilium -f

# === 查看特定Pod的日志 ===
kubectl logs -n kube-system <cilium-pod-name>

# === 查看前一次运行的日志 ===
kubectl logs -n kube-system <cilium-pod-name> --previous

# === Operator日志 ===
kubectl logs -n kube-system deployment/cilium-operator --tail=100

# === 搜索特定关键词 ===
kubectl logs -n kube-system -l k8s-app=cilium | grep -i "error\|fatal\|warning"

# === 搜索OCI相关日志 ===
kubectl logs -n kube-system -l k8s-app=cilium | grep -i oci

# === 搜索IPAM相关日志 ===
kubectl logs -n kube-system deployment/cilium-operator | grep -i "ipam\|vnic\|ip allocation"

# === Hubble Relay日志 ===
kubectl logs -n kube-system deployment/hubble-relay --tail=50

# === 导出所有日志 ===
kubectl logs -n kube-system -l k8s-app=cilium --all-containers=true \
  > cilium-logs-$(date +%Y%m%d-%H%M%S).log
```

### 4.2 事件查看

```bash
# === 查看所有事件（按时间排序） ===
kubectl get events -n kube-system --sort-by='.lastTimestamp'

# === 查看最近的事件 ===
kubectl get events -n kube-system --sort-by='.lastTimestamp' | tail -20

# === 查看Warning事件 ===
kubectl get events -n kube-system --field-selector type=Warning

# === 查看特定Pod的事件 ===
kubectl get events -n kube-system --field-selector involvedObject.name=<pod-name>

# === 持续监控事件 ===
kubectl get events -n kube-system --watch

# === 查看所有Namespace的事件 ===
kubectl get events --all-namespaces --sort-by='.lastTimestamp' | tail -30
```

### 4.3 资源诊断

```bash
# === 查看节点资源使用 ===
kubectl top nodes

# === 查看Pod资源使用 ===
kubectl top pods -n kube-system

# === 查看Cilium Pod资源使用 ===
kubectl top pods -n kube-system -l k8s-app=cilium

# === 查看节点详细信息 ===
kubectl describe node <node-name>

# === 查看节点容量和分配 ===
kubectl describe node <node-name> | grep -A 5 "Allocated resources"

# === 检查磁盘使用 ===
for node in $(kubectl get nodes -o name); do
  echo "=== $node ==="
  kubectl debug $node -it --image=ubuntu -- df -h
done
```

### 4.4 网络诊断

```bash
# === 检查节点路由表 ===
ssh <node-ip> "ip route show"

# === 检查节点路由表（特定table） ===
ssh <node-ip> "ip route show table 210"

# === 检查节点网络接口 ===
ssh <node-ip> "ip addr show"

# === 检查节点iptables规则 ===
ssh <node-ip> "sudo iptables -L -n -v | head -50"

# === 检查节点eBPF挂载 ===
ssh <node-ip> "mount | grep bpf"

# === 查看eBPF maps ===
ssh <node-ip> "sudo ls -la /sys/fs/bpf/tc/globals/"

# === 使用cilium-dbg检查节点状态 ===
kubectl exec -n kube-system <cilium-pod> -- cilium-dbg status

# === 检查endpoint列表 ===
kubectl exec -n kube-system <cilium-pod> -- cilium-dbg endpoint list

# === 检查endpoint详情 ===
kubectl exec -n kube-system <cilium-pod> -- cilium-dbg endpoint get <endpoint-id>

# === 检查BPF policy ===
kubectl exec -n kube-system <cilium-pod> -- cilium-dbg bpf policy get <endpoint-id>
```

### 4.5 生成完整诊断包

```bash
# === 使用Cilium CLI生成sysdump ===
cilium sysdump

# === 指定输出目录 ===
cilium sysdump --output-directory /tmp/cilium-sysdump

# === 包含更多调试信息 ===
cilium sysdump --debug

# === 手动收集诊断信息 ===
mkdir -p cilium-debug-$(date +%Y%m%d-%H%M%S)
cd cilium-debug-$(date +%Y%m%d-%H%M%S)

# 收集基础信息
kubectl version > kubectl-version.txt
kubectl get nodes -o wide > nodes.txt
kubectl get pods -A -o wide > pods-all.txt

# 收集Cilium配置
kubectl get configmap -n kube-system cilium-config -o yaml > cilium-config.yaml
helm get values cilium -n kube-system --all > helm-values.yaml

# 收集CRD
kubectl get ciliumnodes -o yaml > ciliumnodes.yaml
kubectl get ciliumendpoints -A -o yaml > ciliumendpoints.yaml

# 收集日志
kubectl logs -n kube-system -l k8s-app=cilium --all-containers=true > cilium-logs.txt
kubectl logs -n kube-system deployment/cilium-operator > operator-logs.txt

# 收集事件
kubectl get events -A --sort-by='.lastTimestamp' > events.txt

# 打包
cd ..
tar czf cilium-debug-$(date +%Y%m%d-%H%M%S).tar.gz cilium-debug-$(date +%Y%m%d-%H%M%S)/
```

### 4.6 Operator特定问题诊断

```bash
# === 检查Operator镜像是否支持OCI IPAM ===
# 列出Operator容器中的二进制文件
kubectl exec -n kube-system deployment/cilium-operator -- ls -la /usr/bin/

# 应该看到 cilium-operator-oci 文件
# 如果只有 cilium-operator-generic，说明镜像不正确

# === 验证Operator版本和构建信息 ===
kubectl exec -n kube-system deployment/cilium-operator -- \
  cilium-operator-oci --version

# === 检查Operator环境变量 ===
kubectl exec -n kube-system deployment/cilium-operator -- env | grep -i oci

# === 测试OCI API访问（从Operator Pod） ===
kubectl exec -n kube-system deployment/cilium-operator -- \
  curl -s http://169.254.169.254/opc/v2/instance/ | head -20

# === 检查Operator启动参数 ===
kubectl describe pod -n kube-system -l name=cilium-operator | grep -A 10 "Command:"

# === 重建Operator Pod（使用新镜像） ===
# 1. 删除现有Deployment
kubectl delete deployment cilium-operator -n kube-system

# 2. 等待几秒
sleep 5

# 3. Helm升级（会重新创建Deployment）
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --set operator.image.tag=test-fix4 \
  --force

# 4. 验证新Pod运行正常
kubectl wait --for=condition=Ready pod -l name=cilium-operator \
  -n kube-system --timeout=120s

# 5. 检查日志确认OCI provider已加载
kubectl logs -n kube-system deployment/cilium-operator | grep -i "oci.*init"
```

---

## 5. OCI资源管理

### 5.1 Instance Principal测试

```bash
# === 测试Instance Principal ===
oci iam region list --auth instance_principal

# === 获取当前Instance信息 ===
curl -s http://169.254.169.254/opc/v2/instance/ | jq

# === 获取Instance ID ===
INSTANCE_ID=$(curl -s http://169.254.169.254/opc/v2/instance/ | jq -r '.id')
echo $INSTANCE_ID

# === 获取Instance VNIC信息 ===
curl -s http://169.254.169.254/opc/v2/vnics/ | jq
```

### 5.2 VCN和Subnet管理

```bash
# === 查看VCN ===
oci network vcn get --vcn-id <vcn-ocid>

# === 列出VCN中的Subnets ===
oci network subnet list --compartment-id <compartment-ocid> \
  --vcn-id <vcn-ocid>

# === 查看Subnet详情 ===
oci network subnet get --subnet-id <subnet-ocid>

# === 查看Subnet路由表 ===
ROUTE_TABLE_ID=$(oci network subnet get --subnet-id <subnet-ocid> \
  --query 'data."route-table-id"' --raw-output)
oci network route-table get --rt-id $ROUTE_TABLE_ID

# === 查看安全列表 ===
oci network security-list list --compartment-id <compartment-ocid> \
  --vcn-id <vcn-ocid>
```

### 5.3 VNIC管理

```bash
# === 列出Instance的VNIC Attachments ===
oci compute vnic-attachment list \
  --instance-id <instance-ocid> \
  --compartment-id <compartment-ocid>

# === 获取VNIC详情 ===
oci network vnic get --vnic-id <vnic-ocid>

# === 检查skip-source-dest-check ===
oci network vnic get --vnic-id <vnic-ocid> \
  --query 'data."skip-source-dest-check"'

# === 启用skip-source-dest-check ===
oci network vnic update \
  --vnic-id <vnic-ocid> \
  --skip-source-dest-check true \
  --force

# === 列出VNIC的Private IPs ===
oci network private-ip list --vnic-id <vnic-ocid>

# === 创建Private IP ===
oci network private-ip create \
  --vnic-id <vnic-ocid> \
  --display-name "pod-ip-1"
```

### 5.4 IAM管理

```bash
# === 列出Dynamic Groups ===
oci iam dynamic-group list --compartment-id <tenancy-ocid>

# === 查看Dynamic Group详情 ===
oci iam dynamic-group get --dynamic-group-id <dg-ocid>

# === 创建Dynamic Group ===
oci iam dynamic-group create \
  --name cilium-k8s-nodes \
  --description "Kubernetes nodes running Cilium" \
  --matching-rule "Any {instance.compartment.id = '<compartment-ocid>'}"

# === 列出Policies ===
oci iam policy list --compartment-id <compartment-ocid>

# === 查看Policy详情 ===
oci iam policy get --policy-id <policy-ocid>

# === 创建Policy ===
oci iam policy create \
  --compartment-id <compartment-ocid> \
  --name cilium-ipam-policy \
  --description "Cilium OCI IPAM permissions" \
  --statements file://policy-statements.json
```

---

## 6. 多VNIC操作

### 6.1 创建VNIC

```bash
# === 创建VNIC Attachment ===
oci compute instance attach-vnic \
  --instance-id <instance-ocid> \
  --subnet-id <subnet-ocid> \
  --display-name cilium-vnic-2 \
  --skip-source-dest-check true \
  --wait-for-state ATTACHED

# === 获取新创建的VNIC ID ===
VNIC_ATTACHMENT_ID=$(oci compute vnic-attachment list \
  --instance-id <instance-ocid> \
  --query 'data[?display-name==`cilium-vnic-2`].id' \
  --raw-output | tr -d '[]" ')

VNIC_ID=$(oci compute vnic-attachment get \
  --vnic-attachment-id $VNIC_ATTACHMENT_ID \
  --query 'data."vnic-id"' \
  --raw-output)

echo "New VNIC ID: $VNIC_ID"

# === 获取VNIC的Private IP ===
PRIVATE_IP=$(oci network vnic get \
  --vnic-id $VNIC_ID \
  --query 'data."private-ip"' \
  --raw-output)

echo "VNIC Private IP: $PRIVATE_IP"
```

### 6.2 配置节点网络

```bash
# === 配置网络接口 ===
# 登录到节点
ssh <node-ip>

# 查找新VNIC对应的网络接口
ip link show | grep -E "enp|ens|eth"

# 假设新接口是enp1s0
INTERFACE="enp1s0"
VNIC_IP="10.0.1.56"
GATEWAY="10.0.1.1"

# 配置IP地址
sudo ip addr add ${VNIC_IP}/24 dev $INTERFACE
sudo ip link set $INTERFACE up

# 添加路由表
sudo ip route add default via $GATEWAY dev $INTERFACE table 210

# 添加路由规则
sudo ip rule add from $VNIC_IP table 210
sudo ip rule add from 10.0.1.0/24 table 210

# 验证配置
ip addr show $INTERFACE
ip route show table 210
ip rule show

# === 持久化配置（netplan） ===
sudo tee /etc/netplan/60-vnic2.yaml <<EOF
network:
  version: 2
  ethernets:
    $INTERFACE:
      addresses:
        - ${VNIC_IP}/24
      routes:
        - to: 0.0.0.0/0
          via: $GATEWAY
          table: 210
      routing-policy:
        - from: $VNIC_IP
          table: 210
        - from: 10.0.1.0/24
          table: 210
EOF

# 应用配置
sudo netplan apply
```

### 6.3 验证多VNIC

```bash
# === 创建大量Pods触发多VNIC ===
NODE_NAME="cilium-w1"  # 替换为有多VNIC的节点名

for i in {1..40}; do
  kubectl run test-multi-vnic-$i \
    --image=busybox \
    --overrides="{
      \"spec\": {
        \"nodeSelector\": {
          \"kubernetes.io/hostname\": \"$NODE_NAME\"
        }
      }
    }" \
    -- sleep 3600
done

# === 等待所有Pods运行 ===
kubectl wait --for=condition=Ready pod -l run --timeout=300s

# === 查看Pod IP分配 ===
kubectl get pods -o custom-columns=\
NAME:.metadata.name,\
NODE:.spec.nodeName,\
IP:.status.podIP | grep test-multi-vnic | sort -t. -k3,3n -k4,4n

# === 统计不同Subnet的Pod数量 ===
echo "VNIC1 (10.0.0.x):"
kubectl get pods -o jsonpath='{.items[*].status.podIP}' | \
  tr ' ' '\n' | grep "^10.0.0" | wc -l

echo "VNIC2 (10.0.1.x):"
kubectl get pods -o jsonpath='{.items[*].status.podIP}' | \
  tr ' ' '\n' | grep "^10.0.1" | wc -l

# === 清理测试Pods ===
kubectl delete pods -l run=test-multi-vnic-
```

---

## 7. Hubble操作

### 7.1 安装Hubble CLI

```bash
# === 下载和安装Hubble CLI ===
HUBBLE_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/hubble/master/stable.txt)
HUBBLE_ARCH=amd64
curl -L --remote-name-all \
  https://github.com/cilium/hubble/releases/download/$HUBBLE_VERSION/hubble-linux-${HUBBLE_ARCH}.tar.gz
sudo tar xzvfC hubble-linux-${HUBBLE_ARCH}.tar.gz /usr/local/bin
rm hubble-linux-${HUBBLE_ARCH}.tar.gz

# === 验证安装 ===
hubble version
```

### 7.2 访问Hubble

```bash
# === 使用Cilium CLI配置端口转发 ===
cilium hubble port-forward &

# === 手动配置端口转发 ===
kubectl port-forward -n kube-system svc/hubble-relay 4245:80 &

# === 配置环境变量 ===
export HUBBLE_SERVER=localhost:4245

# === 测试连接 ===
hubble status

# === 访问Hubble UI (Port Forward) ===
kubectl port-forward -n kube-system svc/hubble-ui 12000:80 --address 0.0.0.0

# 浏览器访问: http://localhost:12000

# === 访问Hubble UI (NodePort) ===
# 修改Service为NodePort
kubectl patch svc -n kube-system hubble-ui -p '{"spec":{"type":"NodePort"}}'

# 获取NodePort
NODE_PORT=$(kubectl get svc -n kube-system hubble-ui \
  -o jsonpath='{.spec.ports[0].nodePort}')
echo "Hubble UI available at: http://<any-node-ip>:$NODE_PORT"
```

### 7.3 观察流量

```bash
# === 观察所有流量 ===
hubble observe

# === 观察最近100条流量 ===
hubble observe --last 100

# === 持续观察新流量 ===
hubble observe --follow

# === 观察特定Namespace ===
hubble observe --namespace default

# === 观察特定Pod ===
hubble observe --pod default/test-nginx

# === 观察特定类型流量 ===
hubble observe --type drop        # 被丢弃的包
hubble observe --type trace       # 追踪信息
hubble observe --type l7          # 7层流量

# === 观察特定协议 ===
hubble observe --protocol tcp
hubble observe --protocol udp
hubble observe --protocol icmp
hubble observe --protocol http

# === 观察特定verdict ===
hubble observe --verdict FORWARDED  # 已转发
hubble observe --verdict DROPPED    # 已丢弃
hubble observe --verdict ERROR      # 错误

# === 组合条件 ===
hubble observe \
  --namespace default \
  --pod test-client \
  --protocol tcp \
  --verdict DROPPED

# === 观察两个Pods之间的流量 ===
hubble observe \
  --from-pod default/test-client \
  --to-pod default/test-server

# === 观察到特定IP的流量 ===
hubble observe --to-ip 8.8.8.8

# === 观察到特定端口的流量 ===
hubble observe --to-port 80
hubble observe --to-port 443

# === 观察DNS查询 ===
hubble observe --type l7 --protocol dns

# === 观察HTTP流量 ===
hubble observe --type l7 --protocol http

# === JSON格式输出 ===
hubble observe -o json | jq

# === 紧凑格式输出 ===
hubble observe -o compact

# === 详细格式输出 ===
hubble observe -o dict
```

### 7.4 Hubble Metrics

```bash
# === 查看可用metrics ===
kubectl exec -n kube-system <cilium-pod> -- \
  cilium-dbg metrics list | grep hubble

# === 获取metrics ===
kubectl exec -n kube-system <cilium-pod> -- \
  curl -s localhost:9090/metrics | grep hubble

# === 查看Drop metrics ===
hubble observe --type drop --last 1000 | \
  grep -o "dropped due to [^,]*" | sort | uniq -c | sort -rn

# === 查看流量统计 ===
hubble observe --last 1000 -o json | \
  jq -r '.flow | "\(.source.namespace)/\(.source.pod_name) -> \(.destination.namespace)/\(.destination.pod_name)"' | \
  sort | uniq -c | sort -rn | head -20
```

---

## 8. 性能监控

### 8.1 资源监控

```bash
# === 查看节点资源 ===
kubectl top nodes

# === 查看Cilium Pod资源使用 ===
kubectl top pods -n kube-system -l k8s-app=cilium

# === 持续监控资源使用 ===
watch -n 2 'kubectl top pods -n kube-system -l k8s-app=cilium'

# === 查看节点eBPF map使用情况 ===
kubectl exec -n kube-system <cilium-pod> -- cilium-dbg bpf metrics list
```

### 8.2 IPAM监控

```bash
# === 查看所有节点的IP使用情况 ===
kubectl get ciliumnodes -o custom-columns=\
NODE:.metadata.name,\
USED:.status.ipam.used,\
AVAILABLE:.status.ipam.available,\
LIMIT:.status.ipam.limit

# === 查看特定节点的IP池详情 ===
kubectl get ciliumnode <node-name> -o jsonpath='{.status.ipam}' | jq

# === 监控IP分配速率 ===
watch -n 5 "kubectl get ciliumnodes -o custom-columns=\
NODE:.metadata.name,\
USED:.status.ipam.used,\
AVAILABLE:.status.ipam.available"

# === 查看IP分配历史（从Operator日志） ===
kubectl logs -n kube-system deployment/cilium-operator | \
  grep "IP allocation" | tail -20
```

### 8.3 OCI API调用监控

```bash
# === 查看OCI API调用日志 ===
kubectl logs -n kube-system deployment/cilium-operator | \
  grep -i "oci.*api\|ListVnicAttachments\|CreatePrivateIp"

# === 统计OCI API调用次数 ===
kubectl logs -n kube-system deployment/cilium-operator | \
  grep "OCI API call" | cut -d'"' -f4 | sort | uniq -c | sort -rn

# === 查看OCI API错误 ===
kubectl logs -n kube-system deployment/cilium-operator | \
  grep -i "oci.*error\|401\|429\|500"
```

---

## 9. 日常运维

### 9.1 重启组件

```bash
# === 重启Cilium Agent (DaemonSet) ===
kubectl rollout restart daemonset/cilium -n kube-system

# === 重启Cilium Operator ===
kubectl rollout restart deployment/cilium-operator -n kube-system

# === 重启Hubble Relay ===
kubectl rollout restart deployment/hubble-relay -n kube-system

# === 重启Hubble UI ===
kubectl rollout restart deployment/hubble-ui -n kube-system

# === 重启特定Pod ===
kubectl delete pod -n kube-system <pod-name>

# === 查看重启状态 ===
kubectl rollout status daemonset/cilium -n kube-system
```

### 9.2 配置更新

```bash
# === 更新ConfigMap ===
kubectl edit configmap -n kube-system cilium-config

# === 更新后重启Pods使配置生效 ===
kubectl rollout restart daemonset/cilium -n kube-system

# === 更新Helm配置 ===
helm upgrade cilium ./install/kubernetes/cilium \
  --namespace kube-system \
  --reuse-values \
  --set <key>=<value>

# === 验证配置更新 ===
kubectl get configmap -n kube-system cilium-config -o yaml | grep <key>
```

### 9.3 清理操作

```bash
# === 清理已完成的Pods ===
kubectl delete pods --field-selector=status.phase==Succeeded -A

# === 清理失败的Pods ===
kubectl delete pods --field-selector=status.phase==Failed -A

# === 清理Evicted Pods ===
kubectl get pods -A | grep Evicted | \
  awk '{print $1, $2}' | xargs -n2 kubectl delete pod -n

# === 清理测试Pods ===
kubectl delete pods -l app=test

# === 完全卸载Cilium ===
helm uninstall cilium -n kube-system

# 删除CRDs
kubectl delete crd \
  ciliumnetworkpolicies.cilium.io \
  ciliumclusterwidenetworkpolicies.cilium.io \
  ciliumendpoints.cilium.io \
  ciliumidentities.cilium.io \
  ciliumnodes.cilium.io \
  ciliumexternalworkloads.cilium.io \
  ciliumlocalredirectpolicies.cilium.io \
  ciliumegressgatewaypolicies.cilium.io

# 清理节点上的eBPF程序和maps
for node in $(kubectl get nodes -o name | cut -d'/' -f2); do
  echo "Cleaning up $node..."
  ssh $node "sudo rm -rf /sys/fs/bpf/tc/globals/*"
done
```

---

## 10. 一键脚本

### 10.1 完整状态检查脚本

```bash
#!/bin/bash
# check-cilium-complete.sh - 完整的Cilium健康检查

echo "========================================="
echo "Cilium OCI IPAM Health Check"
echo "========================================="

echo -e "\n=== 1. Cluster Information ==="
kubectl version --short
kubectl get nodes -o wide

echo -e "\n=== 2. Cilium Status ==="
cilium status --wait 2>/dev/null || echo "Cilium CLI not available, skipping..."

echo -e "\n=== 3. Cilium Pods ==="
kubectl get pods -n kube-system -l k8s-app=cilium -o wide

echo -e "\n=== 4. Operator Status ==="
kubectl get pods -n kube-system -l name=cilium-operator -o wide

echo -e "\n=== 5. Hubble Components ==="
kubectl get pods -n kube-system | grep hubble

echo -e "\n=== 6. CiliumNodes IPAM Status ==="
kubectl get ciliumnodes -o custom-columns=\
NODE:.metadata.name,\
USED:.status.ipam.used,\
AVAILABLE:.status.ipam.available,\
LIMIT:.status.ipam.limit

echo -e "\n=== 7. Recent Events (Last 10) ==="
kubectl get events -n kube-system --sort-by='.lastTimestamp' | tail -10

echo -e "\n=== 8. Resource Usage ==="
kubectl top nodes 2>/dev/null || echo "Metrics server not available"
kubectl top pods -n kube-system -l k8s-app=cilium 2>/dev/null || echo "Metrics server not available"

echo -e "\n=== 9. OCI Configuration ==="
kubectl get configmap -n kube-system cilium-config -o yaml | grep -E "oci-|ipam-mode"

echo -e "\n=== 10. Recent Errors in Logs ==="
echo "Cilium Agent errors:"
kubectl logs -n kube-system -l k8s-app=cilium --tail=50 --since=5m 2>/dev/null | \
  grep -i "error\|fatal\|warning" | tail -5

echo -e "\nOperator errors:"
kubectl logs -n kube-system deployment/cilium-operator --tail=50 --since=5m 2>/dev/null | \
  grep -i "error\|fatal\|warning" | tail -5

echo -e "\n========================================="
echo "Health Check Complete"
echo "========================================="
```

### 10.2 网络快速测试脚本

```bash
#!/bin/bash
# test-network-quick.sh - 快速网络连通性测试

echo "========================================="
echo "Quick Network Connectivity Test"
echo "========================================="

# 创建测试Pods
echo -e "\n=== Creating test Pods ==="
kubectl run test-client --image=busybox --restart=Never -- sleep 3600 2>/dev/null || true
kubectl run test-server --image=nginx --restart=Never 2>/dev/null || true

echo "Waiting for Pods to be ready..."
kubectl wait --for=condition=Ready pod/test-client --timeout=60s
kubectl wait --for=condition=Ready pod/test-server --timeout=60s

# 获取IP
SERVER_IP=$(kubectl get pod test-server -o jsonpath='{.status.podIP}')
echo "Test server IP: $SERVER_IP"

# 测试1: Pod → Pod (ICMP)
echo -e "\n=== Test 1: Pod → Pod (ICMP) ==="
kubectl exec test-client -- ping -c 3 $SERVER_IP && echo "✓ PASSED" || echo "✗ FAILED"

# 测试2: Pod → Pod (TCP)
echo -e "\n=== Test 2: Pod → Pod (HTTP) ==="
kubectl exec test-client -- wget -O- http://$SERVER_IP --timeout=5 > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"

# 测试3: Pod → Service
echo -e "\n=== Test 3: Pod → Service ==="
kubectl expose pod test-server --port=80 --name=test-svc 2>/dev/null || true
sleep 2
kubectl exec test-client -- wget -O- http://test-svc --timeout=5 > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"

# 测试4: DNS
echo -e "\n=== Test 4: DNS Resolution ==="
kubectl exec test-client -- nslookup test-svc > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"

# 测试5: Internet
echo -e "\n=== Test 5: Pod → Internet ==="
kubectl exec test-client -- ping -c 3 8.8.8.8 > /dev/null 2>&1 && echo "✓ PASSED" || echo "✗ FAILED"

# 清理
echo -e "\n=== Cleaning up ==="
kubectl delete pod test-client test-server --grace-period=0 --force 2>/dev/null
kubectl delete svc test-svc 2>/dev/null

echo -e "\n========================================="
echo "Network Test Complete"
echo "========================================="
```

### 10.3 日志收集脚本

```bash
#!/bin/bash
# collect-logs.sh - 收集Cilium相关日志用于故障排查

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_DIR="cilium-logs-$TIMESTAMP"

echo "Collecting Cilium logs to $OUTPUT_DIR..."
mkdir -p "$OUTPUT_DIR"

# 收集基础信息
echo "Collecting cluster information..."
kubectl version > "$OUTPUT_DIR/kubectl-version.txt"
kubectl get nodes -o wide > "$OUTPUT_DIR/nodes.txt"
kubectl get pods -A -o wide > "$OUTPUT_DIR/pods-all.txt"

# 收集Cilium配置
echo "Collecting Cilium configuration..."
kubectl get configmap -n kube-system cilium-config -o yaml > "$OUTPUT_DIR/cilium-config.yaml"
helm get values cilium -n kube-system --all > "$OUTPUT_DIR/helm-values.yaml" 2>/dev/null || echo "Helm not available"

# 收集CRDs
echo "Collecting CRDs..."
kubectl get ciliumnodes -o yaml > "$OUTPUT_DIR/ciliumnodes.yaml"
kubectl get ciliumendpoints -A -o yaml > "$OUTPUT_DIR/ciliumendpoints.yaml"

# 收集Pods状态
echo "Collecting Pod status..."
kubectl describe pods -n kube-system -l k8s-app=cilium > "$OUTPUT_DIR/cilium-pods-describe.txt"
kubectl describe deployment -n kube-system cilium-operator > "$OUTPUT_DIR/operator-describe.txt"

# 收集日志
echo "Collecting logs..."
kubectl logs -n kube-system -l k8s-app=cilium --all-containers=true --tail=1000 > "$OUTPUT_DIR/cilium-logs.txt"
kubectl logs -n kube-system deployment/cilium-operator --tail=1000 > "$OUTPUT_DIR/operator-logs.txt"

# 如果有Hubble
kubectl logs -n kube-system deployment/hubble-relay --tail=500 > "$OUTPUT_DIR/hubble-relay-logs.txt" 2>/dev/null

# 收集事件
echo "Collecting events..."
kubectl get events -A --sort-by='.lastTimestamp' > "$OUTPUT_DIR/events.txt"
kubectl get events -n kube-system --sort-by='.lastTimestamp' > "$OUTPUT_DIR/events-kube-system.txt"

# 运行cilium status
echo "Collecting Cilium status..."
cilium status > "$OUTPUT_DIR/cilium-status.txt" 2>/dev/null || echo "Cilium CLI not available"

# 打包
echo "Creating archive..."
tar czf "$OUTPUT_DIR.tar.gz" "$OUTPUT_DIR"

echo "========================================="
echo "Log collection complete!"
echo "Archive: $OUTPUT_DIR.tar.gz"
echo "========================================="
```

### 10.4 OCI资源检查脚本

```bash
#!/bin/bash
# check-oci-resources.sh - 检查OCI资源配置

echo "========================================="
echo "OCI Resources Check"
echo "========================================="

# 获取配置
VCN_ID=$(kubectl get configmap -n kube-system cilium-config -o jsonpath='{.data.oci-vcn-id}')
SUBNET_ID=$(kubectl get configmap -n kube-system cilium-config -o jsonpath='{.data.oci-subnet-ocid}')

echo -e "\n=== Configuration ==="
echo "VCN ID: $VCN_ID"
echo "Subnet ID: $SUBNET_ID"

# 测试Instance Principal
echo -e "\n=== Instance Principal Test ==="
oci iam region list --auth instance_principal > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "✓ Instance Principal working"
else
  echo "✗ Instance Principal failed"
fi

# 获取Instance信息
echo -e "\n=== Instance Information ==="
INSTANCE_ID=$(curl -s http://169.254.169.254/opc/v2/instance/ | jq -r '.id')
echo "Instance ID: $INSTANCE_ID"

# 列出VNICs
echo -e "\n=== VNICs Attached ==="
oci compute vnic-attachment list \
  --instance-id $INSTANCE_ID \
  --auth instance_principal \
  --query 'data[*].{DisplayName:"display-name", State:"lifecycle-state", VnicId:"vnic-id"}' \
  --output table

# 检查VCN
echo -e "\n=== VCN Information ==="
oci network vcn get \
  --vcn-id $VCN_ID \
  --auth instance_principal \
  --query 'data.{Name:"display-name", CIDR:"cidr-block", State:"lifecycle-state"}' \
  --output table

# 检查Subnet
echo -e "\n=== Subnet Information ==="
oci network subnet get \
  --subnet-id $SUBNET_ID \
  --auth instance_principal \
  --query 'data.{Name:"display-name", CIDR:"cidr-block", Available:"available-ipv4-address-count"}' \
  --output table

echo -e "\n========================================="
echo "OCI Resources Check Complete"
echo "========================================="
```

---

## 附录: 常用变量

```bash
# === Kubernetes变量 ===
export KUBECONFIG=~/.kube/config
export NAMESPACE=kube-system

# === OCI变量 ===
export OCI_CLI_AUTH=instance_principal
export VCN_ID="ocid1.vcn.oc1.ap-singapore-1.xxxxx"
export SUBNET_ID="ocid1.subnet.oc1.ap-singapore-1.xxxxx"
export COMPARTMENT_ID="ocid1.compartment.oc1..xxxxx"

# === Cilium变量 ===
export CILIUM_NAMESPACE=kube-system
export HUBBLE_SERVER=localhost:4245

# === 常用命令别名 ===
alias k='kubectl'
alias kgp='kubectl get pods'
alias kgs='kubectl get svc'
alias kgn='kubectl get nodes'
alias kl='kubectl logs'
alias kd='kubectl describe'
alias kc='kubectl config'
alias cil='kubectl -n kube-system get pods -l k8s-app=cilium'
alias cill='kubectl -n kube-system logs -l k8s-app=cilium'
```

---

**文档版本：** 1.0  
**最后更新：** 2025年10月24日  
**维护人：** Dengwei
