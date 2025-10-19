# Cilium OCI IPAM（中文文档）

Oracle 云基础设施 (OCI) IPAM 集成，用于 Cilium。

## 概述

Cilium 的 OCI IPAM 模式实现了与 Oracle 云基础设施网络的原生集成。Pod 不使用 Cilium 的内置 IPAM，而是直接从 OCI VCN（虚拟云网络）子网通过 VNIC（虚拟网络接口卡）分配获得 IP 地址。

## 核心特性

- ✅ **原生 OCI 网络**: Pod 从 OCI VCN 子网获得 IP 地址
- ✅ **动态 VNIC 管理**: 自动创建和附加 VNIC
- ✅ **实例主体认证**: 使用 OCI 实例主体进行安全认证
- ✅ **灵活扩展**: 根据 Pod 需求自动分配 IP
- ✅ **多子网支持**: 在多个子网之间分配 Pod
- ✅ **形状感知限制**: 自动检测实例 VNIC 限制

## 工作原理

```
┌─────────────────────────────────────────────────────────┐
│                    OCI 工作节点                          │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   Pod A      │  │   Pod B      │  │   Pod C      │  │
│  │ 10.0.1.10    │  │ 10.0.1.11    │  │ 10.0.2.10    │  │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │
│         │                 │                 │           │
│         └────────┬────────┘                 │           │
│                  │                          │           │
│         ┌────────▼────────┐       ┌─────────▼────────┐  │
│         │   VNIC 1        │       │   VNIC 2         │  │
│         │   主网卡        │       │   辅助网卡        │  │
│         │   10.0.1.5      │       │   10.0.2.5       │  │
│         │   (eth0)        │       │   (eth1)         │  │
│         └────────┬────────┘       └─────────┬────────┘  │
│                  │                          │           │
└──────────────────┼──────────────────────────┼───────────┘
                   │                          │
                   └──────────┬───────────────┘
                              │
                   ┌──────────▼──────────┐
                   │    OCI VCN          │
                   │    10.0.0.0/16      │
                   │                     │
                   │  ┌──────────────┐   │
                   │  │  子网 1      │   │
                   │  │  10.0.1.0/24 │   │
                   │  └──────────────┘   │
                   │  ┌──────────────┐   │
                   │  │  子网 2      │   │
                   │  │  10.0.2.0/24 │   │
                   │  └──────────────┘   │
                   └─────────────────────┘
```

### IPAM 流程

1. **Pod 创建请求** → Cilium 检测到新 Pod 需要 IP
2. **VNIC 选择** → Cilium 检查现有 VNIC 是否有可用 IP
3. **IP 分配**:
   - 如果有空闲：为现有 VNIC 分配辅助 IP
   - 如果已满：在合适的子网中创建新 VNIC
4. **Pod 网络配置** → 使用分配的 IP 配置 Pod 网络接口
5. **路由更新** → 更新路由表以实现 Pod 连接

## 文档

- **[快速入门指南](quickstart_CN.md)** - 5 步快速入门
- **[故障排查指南](troubleshooting_CN.md)** - 常见问题和解决方案
- **[配置参考](configuration_CN.md)** - 详细配置选项
- **[English Documentation](README.md)** - 英文文档

## 何时使用 OCI IPAM

### ✅ 适合使用 OCI IPAM 的场景:

- 需要 Pod 直接与 OCI 资源（数据库、虚拟机等）通信
- 希望利用 OCI 网络安全组进行 Pod 级别的安全控制
- 组织要求所有 IP 都来自受管理的 VCN 子网
- 需要在 Kubernetes 和非 Kubernetes 工作负载之间保持一致的 IP 地址
- 想要使用 OCI 原生负载均衡器与 Pod IP

### ❌ 考虑替代方案的场景:

- VCN CIDR 较小且需要节省 IP
- 希望 Pod IP 与基础设施完全隔离
- 每个节点需要超过 32 个 IP（每个 VNIC）
- 集群不在 OCI 上运行

## 要求

### 基础设施
- OCI Kubernetes 集群（OKE）或 OCI 计算实例上的自管理 Kubernetes
- Kubernetes 版本 1.23+
- 具有足够 IP 空间的 OCI VCN
- 多个子网（推荐）以提高冗余性

### 权限
- 实例主体或具有适当 IAM 策略的 OCI 配置文件
- 管理 VNIC、私有 IP 和查询 VCN 资源的权限

### Cilium
- Cilium 版本 1.15.2+
- 使用 `ipam_provider_oci` 标签构建

## 快速示例

最小 Helm 配置:

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."

OCIUseInstancePrincipal: true

operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...
```

安装:

```bash
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values values.yaml
```

## 性能考虑

### VNIC 限制

每个 OCI 实例形状都有以下限制:
- **最大 VNIC 数**: 根据形状而异（通常 2-24）
- **每 VNIC 的 IP 数**: 32 个辅助 IP（加 1 个主 IP）

容量示例:
```
VM.Standard.E4.Flex (4 OCPU): 2 VNICs × 32 IPs = 64 个 Pod/节点
BM.Standard.E4.128: 24 VNICs × 32 IPs = 768 个 Pod/节点
```

### 扩展行为

- **首次 IP 分配**: ~2-3 秒（创建 VNIC）
- **同一 VNIC 上的后续 IP**: ~500 毫秒
- **新 VNIC 创建**: ~3-5 秒
- **VNIC 附加**: ~5-10 秒

### 优化提示

1. **预分配 VNIC**: 设置更高的预分配阈值
2. **使用多个子网**: 跨可用性域分散负载
3. **监控 VNIC 使用**: 设置 VNIC 耗尽告警
4. **合适的实例形状**: 选择具有足够 VNIC 限制的形状

## 与其他 IPAM 模式的比较

| 特性 | OCI IPAM | Cluster Pool | Kubernetes Host Scope |
|------|----------|--------------|----------------------|
| IP 来源 | OCI VCN 子网 | Cilium 管理的池 | 节点 PodCIDR |
| Pod 到 OCI 延迟 | 最低 | 中等 | 中等 |
| IP 节省 | 中等 | 高 | 中等 |
| OCI 集成 | 原生 | 无 | 无 |
| 复杂度 | 中等 | 低 | 低 |
| 可扩展性 | 受形状限制 | 无限制 | 受节点限制 |

## 架构细节

### 组件

- **Cilium Operator**: 管理集群的 IPAM
  - 发现 OCI 实例形状和限制
  - 从 VCN 子网分配 IP
  - 根据需要创建和附加 VNIC

- **Cilium Agent**: 在每个节点上运行
  - 通过 CiliumNode CRD 报告节点 IPAM 状态
  - 配置 Pod 网络接口
  - 维护本地 IP 分配状态

- **OCI API 集成**: 
  - 虚拟网络 API 用于 VNIC 管理
  - 计算 API 用于实例查询
  - 资源搜索 API 用于 VCN 发现

### CRD 架构

OCI 的 CiliumNode spec 和 status:

```yaml
apiVersion: cilium.io/v2
kind: CiliumNode
metadata:
  name: node-1
spec:
  oci:
    vcn-id: "ocid1.vcn.oc1.phx.xxx"
    availability-domain: "AD-1"
    subnet-tags:
      environment: production
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
          - "10.0.1.10"
          - "10.0.1.11"
        subnet:
          id: "ocid1.subnet.oc1.phx.xxx"
          cidr: "10.0.1.0/24"
        vcn:
          id: "ocid1.vcn.oc1.phx.xxx"
          cidr-blocks:
            - "10.0.0.0/16"
```

## 安全考虑

### IAM 最佳实践

1. **使用实例主体**: 避免存储凭据
2. **最小权限**: 仅授予所需权限
3. **隔离区间**: 为不同环境使用单独的区间
4. **审计日志**: 为 VNIC 操作启用 OCI 审计日志

### 网络安全

1. **安全列表**: 为子网应用安全列表
2. **网络安全组**: 使用 NSG 进行细粒度的 Pod 安全控制
3. **私有子网**: 为 Pod IP 使用私有子网
4. **路由表**: 为 Pod 流量配置适当的路由

## 获取帮助

- **文档**: 参见 [quickstart_CN.md](quickstart_CN.md) 和 [troubleshooting_CN.md](troubleshooting_CN.md)
- **日志**: `kubectl -n kube-system logs deployment/cilium-operator`
- **状态**: `kubectl get ciliumnodes -o yaml`
- **GitHub Issues**: 报告错误和请求功能
- **English Docs**: [README.md](README.md), [quickstart.md](quickstart.md)

## 贡献

欢迎贡献！请参见主 [Cilium 贡献指南](../../../CONTRIBUTING.md)。

## 许可证

Apache License 2.0 - 参见 [LICENSE](../../../LICENSE)
