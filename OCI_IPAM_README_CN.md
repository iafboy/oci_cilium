# Cilium OCI IPAM - 中文说明

**版本**: Cilium v1.15.2  
**集成完成日期**: 2025年10月19日  
**源代码基础**: xmltiger/Cilium-for-OCI (基于 Cilium v1.13)

---

## 📌 快速导航

- [什么是 OCI IPAM？](#什么是-oci-ipam)
- [核心特性](#核心特性)
- [快速开始](#快速开始)
- [架构设计](#架构设计)
- [配置说明](#配置说明)
- [文档索引](#文档索引)
- [常见问题](#常见问题)

---

## 什么是 OCI IPAM？

OCI IPAM 是 Cilium 的 IPAM (IP 地址管理) 提供者，专为 Oracle 云基础设施 (OCI) 设计。它允许 Kubernetes Pod 直接使用 OCI VCN (虚拟云网络) 的 IP 地址，而不是使用 Overlay 网络。

### 工作原理

```
┌─────────────────────────────────────────────┐
│         Kubernetes 集群 (OCI)                │
│                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │  Pod A   │  │  Pod B   │  │  Pod C   │  │
│  │10.0.1.10 │  │10.0.1.11 │  │10.0.2.10 │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
│       │             │             │         │
│       └──────┬──────┘             │         │
│              │                    │         │
│       ┌──────▼──────┐      ┌──────▼──────┐ │
│       │  VNIC 1     │      │  VNIC 2     │ │
│       │  eth0 (主)  │      │  eth1 (辅)  │ │
│       │  10.0.1.5   │      │  10.0.2.5   │ │
│       └──────┬──────┘      └──────┬──────┘ │
│              │                    │         │
└──────────────┼────────────────────┼─────────┘
               │                    │
               └────────┬───────────┘
                        │
               ┌────────▼────────┐
               │   OCI VCN       │
               │   10.0.0.0/16   │
               │                 │
               │ ┌────────────┐  │
               │ │ 子网 1     │  │
               │ │10.0.1.0/24 │  │
               │ └────────────┘  │
               │ ┌────────────┐  │
               │ │ 子网 2     │  │
               │ │10.0.2.0/24 │  │
               │ └────────────┘  │
               └─────────────────┘
```

**关键概念**:
- **VNIC (虚拟网络接口卡)**: OCI 的网络接口，每个可以有 1 个主 IP + 32 个辅助 IP
- **辅助 IP**: 分配给 Pod 的 IP 地址
- **VCN (虚拟云网络)**: OCI 的私有网络，类似 AWS VPC
- **子网**: VCN 内的 IP 地址段

---

## 核心特性

### ✅ 原生 OCI 网络集成
- Pod 使用 OCI VCN 原生 IP，无需 Overlay
- 直接访问 OCI 服务（数据库、对象存储等）
- 延迟更低，性能更好

### ✅ 动态 IP 管理
- 根据 Pod 需求自动分配 IP
- 自动创建和附加 VNIC
- 智能选择子网

### ✅ 双认证模式
- **实例主体** (Instance Principal) - 推荐，无需凭据
- **配置文件** (Config File) - 使用 API 密钥

### ✅ 形状感知
- 自动检测实例形状的 VNIC 限制
- 支持从 VM 到裸金属的所有形状
- 动态调整容量

### ✅ 高可用性
- 支持多子网
- 跨可用性域分布
- 自动故障转移

---

## 快速开始

### 前提条件

```bash
✅ OCI 上的 Kubernetes 集群 (OKE 或自管理)
✅ Kubernetes 1.23+
✅ Helm 3.0+
✅ VCN 具有足够的 IP 空间
✅ 正确配置的 IAM 权限
```

### 1. 获取 VCN OCID

```bash
# 使用 OCI CLI
oci network vcn list \
  --compartment-id <your-compartment-ocid> \
  --display-name <your-vcn-name> \
  --query 'data[0].id' \
  --raw-output

# 输出示例:
# ocid1.vcn.oc1.phx.aaaaaaaa...
```

### 2. 设置 IAM 策略

创建动态组:
```
规则: ANY {instance.compartment.id = 'ocid1.compartment.oc1..xxx'}
```

创建策略:
```
Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <name>
Allow dynamic-group cilium-oci-ipam to use subnets in compartment <name>
Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <name>
Allow dynamic-group cilium-oci-ipam to use virtual-network-family in compartment <name>
Allow dynamic-group cilium-oci-ipam to read instance-family in compartment <name>
```

### 3. 创建 Helm Values

创建 `cilium-oci-values.yaml`:

```yaml
# 基础配置
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."  # 替换为您的 VCN OCID

# 认证配置
OCIUseInstancePrincipal: true

# Operator 配置
operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...  # 必需！

# 网络配置
ipv4NativeRoutingCIDR: "10.0.0.0/16"  # 替换为您的 VCN CIDR
tunnel: disabled
autoDirectNodeRoutes: true
enableIPv4Masquerade: true

# 可观察性（可选）
hubble:
  enabled: true
  relay:
    enabled: true
  ui:
    enabled: true
```

### 4. 安装 Cilium

```bash
# 添加 Helm 仓库
helm repo add cilium https://helm.cilium.io/
helm repo update

# 安装
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values cilium-oci-values.yaml

# 等待部署完成
kubectl -n kube-system rollout status deployment/cilium-operator
kubectl -n kube-system rollout status daemonset/cilium
```

### 5. 验证安装

```bash
# 检查 Cilium 状态
kubectl -n kube-system get pods -l k8s-app=cilium

# 检查 IPAM 模式
kubectl -n kube-system get cm cilium-config -o yaml | grep -i ipam

# 检查 CiliumNode
kubectl get ciliumnodes

# 检查 OCI 状态
kubectl get ciliumnode <node-name> -o yaml | grep -A 20 oci
```

### 6. 测试 Pod 网络

```bash
# 创建测试 Pod
kubectl run test-pod --image=nginx

# 检查 Pod IP (应该在 VCN CIDR 范围内)
kubectl get pod test-pod -o wide

# 测试连接
kubectl exec test-pod -- curl https://www.google.com
```

🎉 **完成！** 您的 OCI IPAM 现在已经运行！

---

## 架构设计

### 组件架构

```
┌──────────────────────────────────────────────────────┐
│                  Cilium Operator                      │
│  ┌────────────────────────────────────────────────┐  │
│  │           OCI IPAM Allocator                   │  │
│  │  ┌──────────────┐  ┌────────────────────────┐ │  │
│  │  │ OCI Client   │  │   VNIC Manager         │ │  │
│  │  │ - Auth       │  │   - 创建 VNIC          │ │  │
│  │  │ - API Calls  │  │   - 分配 IP            │ │  │
│  │  └──────────────┘  │   - 管理生命周期       │ │  │
│  │  ┌──────────────┐  └────────────────────────┘ │  │
│  │  │  Metadata    │                              │  │
│  │  │  Client      │                              │  │
│  │  └──────────────┘                              │  │
│  └────────────────────────────────────────────────┘  │
└─────────────────┬────────────────────────────────────┘
                  │ OCI SDK
         ┌────────▼──────────┐
         │     OCI APIs       │
         │  - Virtual Network │
         │  - Compute         │
         │  - Identity        │
         └────────┬───────────┘
                  │
    ┌─────────────▼──────────────┐
    │      CiliumNode CRD        │
    │  ┌──────────────────────┐  │
    │  │  Spec.OCI            │  │
    │  │  - VCN ID            │  │
    │  │  - Subnet Tags       │  │
    │  └──────────────────────┘  │
    │  ┌──────────────────────┐  │
    │  │  Status.OCI          │  │
    │  │  - VNICs             │  │
    │  │  - VNIC Limits       │  │
    │  │  - IP Addresses      │  │
    │  └──────────────────────┘  │
    └────────────────────────────┘
```

### IPAM 流程

```
1. Pod 创建请求
   ↓
2. Cilium 检测需要 IP
   ↓
3. 检查节点的 CiliumNode CRD
   ↓
4. OCI IPAM Allocator 决策:
   ├─ 有空闲 VNIC? → 分配辅助 IP (500ms)
   │   ↓
   │   更新 CiliumNode Status
   │   ↓
   │   配置 Pod 网络
   │
   └─ 无空闲 VNIC? → 创建新 VNIC (5-10s)
       ↓
       附加到实例
       ↓
       分配主 IP
       ↓
       更新 CiliumNode Status
       ↓
       配置 Pod 网络
```

---

## 配置说明

### 必需参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `ipam.mode` | 必须设为 "oci" | `oci` |
| `oci.vcnId` | VCN 的 OCID | `ocid1.vcn.oc1.phx.xxx` |
| `operator.extraArgs[--oci-vcn-id]` | Operator 需要的 VCN ID | 同上 |

### 认证参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `OCIUseInstancePrincipal` | 使用实例主体认证 | `true` |
| `oci.configPath` | 配置文件路径 | `/root/.oci/config` |

### 高级参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `oci.subnetTags` | 子网标签过滤 | `{}` |
| `oci.vnicPreAllocationThreshold` | VNIC 预分配阈值 | `8` |
| `oci.maxIPsPerVNIC` | 每 VNIC 最大 IP 数 | `32` |
| `oci.maxVNICsPerNode` | 每节点最大 VNIC 数 | 自动检测 |

### 示例配置

**最小配置**:
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
```

**生产配置**:
```yaml
ipam:
  mode: "oci"
oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.xxx"
  subnetTags:
    environment: production
  vnicPreAllocationThreshold: 16
OCIUseInstancePrincipal: true
operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.xxx
ipv4NativeRoutingCIDR: "10.0.0.0/16"
tunnel: disabled
autoDirectNodeRoutes: true
hubble:
  enabled: true
```

详细配置请参考: [配置参考文档](Documentation/network/oci/configuration_CN.md)

---

## 文档索引

### 📚 用户文档

| 文档 | 说明 | 语言 |
|------|------|------|
| [README.md](Documentation/network/oci/README.md) | OCI IPAM 总览 | 🇬🇧 英文 |
| [README_CN.md](Documentation/network/oci/README_CN.md) | OCI IPAM 总览 | 🇨🇳 中文 |
| [quickstart.md](Documentation/network/oci/quickstart.md) | 快速入门指南 | 🇬🇧 英文 |
| [quickstart_CN.md](Documentation/network/oci/quickstart_CN.md) | 快速入门指南 | 🇨🇳 中文 |
| [troubleshooting.md](Documentation/network/oci/troubleshooting.md) | 故障排查指南 | 🇬🇧 英文 |
| [troubleshooting_CN.md](Documentation/network/oci/troubleshooting_CN.md) | 故障排查指南 | 🇨🇳 中文 |
| [configuration.md](Documentation/network/oci/configuration.md) | 配置参考 | 🇬🇧 英文 |
| [configuration_CN.md](Documentation/network/oci/configuration_CN.md) | 配置参考 | 🇨🇳 中文 |

### 🔧 开发者文档

| 文档 | 说明 |
|------|------|
| [OCI_IPAM_REVIEW_REPORT.md](OCI_IPAM_REVIEW_REPORT.md) | 代码审查报告 (英文) |
| [OCI_IPAM_CODE_AUDIT_REPORT_CN.md](OCI_IPAM_CODE_AUDIT_REPORT_CN.md) | 完整审核报告 (中文) |
| [OCI_IPAM_INTEGRATION_SUMMARY.md](OCI_IPAM_INTEGRATION_SUMMARY.md) | 集成摘要 |
| [OCI_GENERATED_FILES_README.md](OCI_GENERATED_FILES_README.md) | 自动生成文件说明 |

---

## 常见问题

### Q1: OCI IPAM 与其他 IPAM 模式有什么区别？

**A**: 

| 特性 | OCI IPAM | Cluster Pool | Kubernetes Host Scope |
|------|----------|--------------|----------------------|
| IP 来源 | OCI VCN 子网 | Cilium 管理池 | 节点 PodCIDR |
| 网络延迟 | 最低 | 中等 | 中等 |
| OCI 集成 | 原生 | 无 | 无 |
| 复杂度 | 中等 | 低 | 低 |
| 可扩展性 | 受形状限制 | 无限制 | 受节点限制 |

### Q2: 每个节点能运行多少个 Pod？

**A**: 容量 = (最大 VNIC 数) × (每 VNIC IP 数)

示例:
- **VM.Standard.E4.Flex (2 OCPU)**: 2 VNICs × 32 IPs = **64 Pods**
- **VM.Standard.E4.Flex (8 OCPU)**: 4 VNICs × 32 IPs = **128 Pods**
- **BM.Standard.E4.128**: 24 VNICs × 32 IPs = **768 Pods**

### Q3: 为什么需要手动指定 VCN ID？

**A**: OCI 实例元数据服务不提供 VCN ID，只能通过 VNC 或子网 OCID 查询。为简化配置，要求手动指定。

### Q4: 实例主体认证和配置文件认证哪个更好？

**A**: **推荐实例主体**:
- ✅ 无需存储凭据
- ✅ 自动轮换
- ✅ 更安全
- ✅ 部署简单

配置文件适用于：
- 测试环境
- 无法使用实例主体的场景

### Q5: Pod 启动很慢怎么办？

**A**: 首次创建 VNIC 需要 5-10 秒。优化方法:
```yaml
oci:
  vnicPreAllocationThreshold: 16  # 增加预分配
  maxParallelAllocations: 5       # 并行分配
```

### Q6: 如何查看 OCI IPAM 状态？

**A**:
```bash
# 查看 CiliumNode
kubectl get ciliumnode <node-name> -o yaml

# 查看 Operator 日志
kubectl -n kube-system logs deployment/cilium-operator

# 查看 IPAM 事件
kubectl get events --all-namespaces | grep -i ipam
```

### Q7: 支持 IPv6 吗？

**A**: 当前版本仅支持 IPv4。IPv6 支持在规划中。

### Q8: 可以动态更改子网吗？

**A**: 不可以。VNIC 创建后绑定到特定子网，无法更改。如需使用不同子网，需要创建新 VNIC。

### Q9: 如何限制使用特定子网？

**A**: 使用子网标签:
```yaml
oci:
  subnetTags:
    environment: production
    tier: app
```
只有匹配所有标签的子网才会被使用。

### Q10: 出现 "no available subnets" 错误怎么办？

**A**: 
1. 检查子网可用 IP: `oci network subnet get --subnet-id <id>`
2. 添加更多子网到 VCN
3. 释放未使用的 IP
4. 检查子网标签是否正确

更多故障排查: [troubleshooting_CN.md](Documentation/network/oci/troubleshooting_CN.md)

---

## 性能特性

### 容量规划

| 实例类型 | VNIC 数 | 每节点 Pod | 适用场景 |
|----------|---------|-----------|----------|
| VM.Standard.E4.Flex (2 OCPU) | 2 | 64 | 开发/测试 |
| VM.Standard.E4.Flex (8 OCPU) | 4 | 128 | 生产 - 小型 |
| VM.Standard.E3.Flex (16 OCPU) | 8 | 256 | 生产 - 中型 |
| BM.Standard.E4.128 | 24 | 768 | 生产 - 大型 |

### 延迟基准

| 操作 | 延迟 |
|------|------|
| 分配 IP (现有 VNIC) | ~500ms |
| 创建新 VNIC | ~3-5s |
| 附加 VNIC 到实例 | ~5-10s |
| Pod 到 Pod (同节点) | <1ms |
| Pod 到 Pod (跨节点) | ~1-2ms |

---

## 最佳实践

### ✅ 推荐做法

1. **使用实例主体认证**
   ```yaml
   OCIUseInstancePrincipal: true
   ```

2. **规划足够的 IP 空间**
   - 每个节点预留 100+ IP
   - 使用多个子网
   - 监控子网可用 IP

3. **启用可观察性**
   ```yaml
   hubble:
     enabled: true
   ```

4. **设置合适的预分配阈值**
   ```yaml
   oci:
     vnicPreAllocationThreshold: 16
   ```

5. **使用子网标签管理**
   ```yaml
   oci:
     subnetTags:
       environment: production
   ```

### ❌ 避免的做法

1. ❌ 不要使用太小的子网 (如 /28)
2. ❌ 不要在生产环境使用配置文件认证
3. ❌ 不要忽略 VNIC 限制
4. ❌ 不要忘记设置 `--oci-vcn-id` 参数
5. ❌ 不要手动修改 `zz_generated.*.go` 文件

---

## 获取帮助

### 📖 文档
- [快速入门](Documentation/network/oci/quickstart_CN.md)
- [故障排查](Documentation/network/oci/troubleshooting_CN.md)
- [配置参考](Documentation/network/oci/configuration_CN.md)

### 🐛 问题报告
- GitHub Issues: 报告 bug 和功能请求
- 包含完整日志和配置

### 💬 社区
- Cilium Slack: 实时讨论
- 邮件列表: 长期讨论

### 📊 监控
```bash
# 查看日志
kubectl -n kube-system logs deployment/cilium-operator -f

# 查看状态
kubectl get ciliumnodes -o wide

# 导出诊断信息
cilium-dbg status
```

---

## 贡献

欢迎贡献！请参见:
- [Cilium 贡献指南](CONTRIBUTING.md)
- [开发者文档](OCI_IPAM_CODE_AUDIT_REPORT_CN.md)

---

## 许可证

Apache License 2.0 - 参见 [LICENSE](LICENSE)

---

**维护者**: SEHUB 团队  
**最后更新**: 2025年10月19日  
**版本**: 1.0
