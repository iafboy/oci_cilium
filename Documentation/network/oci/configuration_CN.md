# OCI IPAM 配置参考

本文件详细介绍 Cilium OCI IPAM 所有可用配置项及其含义。

## 必需参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `ipam.mode` | IPAM 模式，必须为 `oci` | `oci` |
| `oci.enabled` | 启用 OCI IPAM | `true` |
| `oci.vcnId` | VCN 的 OCID，必填 | `ocid1.vcn.oc1.phx.aaaaaa...` |
| `operator.extraArgs[--oci-vcn-id]` | Operator 启动参数，指定 VCN | `--oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...` |

## 认证相关

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `OCIUseInstancePrincipal` | 是否使用实例主体认证 | `true` |
| `oci.configPath` | OCI 配置文件路径（如不使用实例主体） | `/root/.oci/config` |

## 网络相关

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `ipv4NativeRoutingCIDR` | VCN 的 CIDR | `10.0.0.0/16` |
| `tunnel` | 隧道模式，OCI 推荐关闭 | `disabled` |
| `autoDirectNodeRoutes` | 启用节点间直连路由 | `true` |
| `enableIPv4Masquerade` | 启用 IPv4 伪装 | `true` |

## OCI IPAM 高级参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `oci.subnetTags` | 子网标签过滤，选择用于 Pod 的子网 | `{}` |
| `oci.vnicPreAllocationThreshold` | 每节点预分配 VNIC 阈值 | `8` |
| `oci.maxIPsPerVNIC` | 每 VNIC 最大 IP 数 | `32` |
| `oci.maxVNICsPerNode` | 每节点最大 VNIC 数 | 由实例形状自动检测 |
| `oci.maxParallelAllocations` | 并行 IP 分配数 | `5` |

## Operator 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `operator.replicas` | Operator 副本数 | `1` |
| `operator.resources` | Operator 资源限制 | `{}` |
| `operator.extraArgs` | 其他启动参数 | `[]` |

## Hubble 可观察性

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `hubble.enabled` | 启用 Hubble | `true` |
| `hubble.relay.enabled` | 启用 Hubble Relay | `true` |
| `hubble.ui.enabled` | 启用 Hubble UI | `true` |

## 安全相关

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `policyEnforcementMode` | 策略执行模式 | `default` |
| `bpf.hostRouting` | 启用主机路由 | `true` |
| `mtu` | 网络 MTU | `9000`（如支持巨型帧） |

## 配置示例

```yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."
  subnetTags:
    environment: production
    tier: app
  vnicPreAllocationThreshold: 16
  maxIPsPerVNIC: 32
  maxParallelAllocations: 5

OCIUseInstancePrincipal: true
operator:
  replicas: 2
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...

ipv4NativeRoutingCIDR: "10.0.0.0/16"
tunnel: disabled
autoDirectNodeRoutes: true
enableIPv4Masquerade: true
hubble:
  enabled: true
  relay:
    enabled: true
  ui:
    enabled: true
policyEnforcementMode: "default"
bpf:
  hostRouting: true
mtu: 9000
```

## 参数说明

### ipam.mode
- 必须设置为 `oci`，否则不会启用 OCI IPAM。

### oci.vcnId
- 必填，指定用于 Pod IP 分配的 VCN。

### oci.subnetTags
- 通过标签过滤子网，仅在匹配标签的子网中分配 VNIC。

### vnicPreAllocationThreshold
- 控制每节点预分配的 VNIC 数，提升高并发场景下的分配速度。

### maxIPsPerVNIC
- 每个 VNIC 最大辅助 IP 数，OCI 限制为 32。

### maxVNICsPerNode
- 每节点最大 VNIC 数，受实例形状限制。

### maxParallelAllocations
- 并行分配 IP 的最大数，提升大规模 Pod 启动速度。

### operator.extraArgs
- 需包含 `--oci-vcn-id` 参数。

### enableIPv4Masquerade
- 启用后，Pod 流量可访问互联网。

### mtu
- 推荐设置为 9000（如支持巨型帧），否则使用默认 1500。

## 常见配置场景

### 多子网高可用
```yaml
oci:
  subnetTags:
    environment: production
    zone: ad-1
```

### 限制每节点 Pod 数
```yaml
oci:
  maxVNICsPerNode: 4
  maxIPsPerVNIC: 32
# 节点最大 Pod 数 = 4 × 32 = 128
```

### 使用配置文件认证
```yaml
OCIUseInstancePrincipal: false
oci:
  configPath: "/root/.oci/config"
```

## 配置验证

安装前可通过以下命令验证配置:

```bash
helm template cilium cilium/cilium --values values.yaml --debug
```

## 参考文档

- [主 README](README_CN.md)
- [快速入门](quickstart_CN.md)
- [故障排查](troubleshooting_CN.md)
- [English Documentation](configuration.md)
