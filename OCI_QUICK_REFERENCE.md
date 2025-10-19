# 🚀 OCI IPAM 快速参考卡片

**Cilium v1.15.2 OCI IPAM 集成**

---

## 📁 文档快速导航

### 🎯 我想要...

| 需求 | 文档 | 位置 |
|------|------|------|
| **快速了解 OCI IPAM** | `OCI_IPAM_README_CN.md` | 根目录 |
| **5分钟部署** | `quickstart_CN.md` | `Documentation/network/oci/` |
| **解决部署问题** | `troubleshooting_CN.md` | `Documentation/network/oci/` |
| **调整配置** | `configuration_CN.md` | `Documentation/network/oci/` |
| **理解代码架构** | `OCI_IPAM_INTEGRATION_SUMMARY.md` | 根目录 |
| **查看审核报告** | `OCI_IPAM_CODE_AUDIT_REPORT_CN.md` | 根目录 |
| **查找所有文档** | `OCI_DOCUMENTATION_INDEX.md` | 根目录 |

---

## ⚡ 最小部署配置

```yaml
# values.yaml
ipam:
  mode: "oci"

oci:
  enabled: true
  vcnId: "ocid1.vcn.oc1.phx.aaaaaa..."  # ⚠️ 必填

OCIUseInstancePrincipal: true

operator:
  extraArgs:
    - --oci-vcn-id=ocid1.vcn.oc1.phx.aaaaaa...  # ⚠️ 必填
```

**部署命令**:
```bash
helm install cilium cilium/cilium \
  --version 1.15.2 \
  --namespace kube-system \
  --values values.yaml
```

---

## 🔧 常用命令

### 检查状态
```bash
# Cilium 状态
kubectl -n kube-system get pods -l k8s-app=cilium

# CiliumNode 状态
kubectl get ciliumnodes

# OCI VNIC 信息
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnics}' | jq
```

### 查看日志
```bash
# Operator 日志 (IPAM 分配)
kubectl -n kube-system logs deployment/cilium-operator

# Agent 日志
kubectl -n kube-system logs ds/cilium
```

### 故障诊断
```bash
# 检查 IPAM 模式
kubectl -n kube-system get cm cilium-config -o yaml | grep ipam

# 查看 IPAM 事件
kubectl get events -A | grep -i ipam

# 检查 VNIC 限制
kubectl get ciliumnode <node> -o jsonpath='{.status.oci.vnic-limits}'
```

---

## 🚨 常见问题快速修复

| 问题 | 快速修复 |
|------|----------|
| **Pod 无法获取 IP** | 检查子网可用 IP: `oci network subnet get --subnet-id <id>` |
| **权限被拒绝** | 验证 IAM 策略和实例主体动态组 |
| **VCN ID 未找到** | 在 `oci.vcnId` 和 `--oci-vcn-id` 中设置 |
| **VNIC 限制达到** | 使用更大的实例形状或添加更多节点 |

详细故障排查: `Documentation/network/oci/troubleshooting_CN.md`

---

## 📊 容量规划

| 实例形状 | 最大 VNIC | 每节点最大 Pod |
|----------|-----------|---------------|
| VM.Standard.E4.Flex (2 OCPU) | 2 | 64 |
| VM.Standard.E4.Flex (8 OCPU) | 4 | 128 |
| BM.Standard.E4.128 | 24 | 768 |

**计算公式**: 容量 = VNIC 数 × 32 IP/VNIC

---

## 🔑 必需的 IAM 策略

```hcl
# 创建动态组
规则: ANY {instance.compartment.id = 'ocid1.compartment.oc1..xxx'}

# 授予权限
Allow dynamic-group cilium-oci-ipam to manage vnics in compartment <name>
Allow dynamic-group cilium-oci-ipam to use subnets in compartment <name>
Allow dynamic-group cilium-oci-ipam to use network-security-groups in compartment <name>
Allow dynamic-group cilium-oci-ipam to use virtual-network-family in compartment <name>
Allow dynamic-group cilium-oci-ipam to read instance-family in compartment <name>
```

---

## ✅ 验证清单

部署后验证:

```bash
# ✅ Cilium Pod 运行
kubectl -n kube-system get pods -l k8s-app=cilium

# ✅ IPAM 模式为 oci
kubectl -n kube-system get cm cilium-config -o yaml | grep "ipam: oci"

# ✅ CiliumNode 有 OCI 状态
kubectl get ciliumnode <node> -o yaml | grep -A 5 "oci:"

# ✅ Pod 获得 VCN IP
kubectl get pods -A -o wide

# ✅ Pod 网络连接
kubectl run test --image=busybox -it --rm -- ping 8.8.8.8
```

---

## 📈 性能优化

```yaml
# 高性能配置
oci:
  vnicPreAllocationThreshold: 32  # 激进预分配
  maxParallelAllocations: 5       # 并行创建

operator:
  replicas: 2  # 多副本
```

---

## 🌐 文档语言

| 内容 | 英文 | 中文 |
|------|------|------|
| 总览 | README.md | README_CN.md |
| 快速入门 | quickstart.md | quickstart_CN.md |
| 配置 | configuration.md | configuration_CN.md |
| 故障排查 | troubleshooting.md | troubleshooting_CN.md |

所有文档位于: `Documentation/network/oci/`

---

## 📞 获取帮助

1. 📖 查阅文档: `OCI_DOCUMENTATION_INDEX.md`
2. 🐛 报告问题: GitHub Issues
3. 💬 讨论交流: Cilium Slack

---

## 🎯 核心概念

| 术语 | 说明 |
|------|------|
| **VNIC** | 虚拟网络接口卡，OCI 的网络接口 |
| **辅助 IP** | 分配给 Pod 的 IP 地址 (每 VNIC 最多 32 个) |
| **VCN** | 虚拟云网络，类似 AWS VPC |
| **实例主体** | 无需凭据的 OCI 认证方式（推荐） |

---

## 🔗 快速链接

- 完整文档索引: `OCI_DOCUMENTATION_INDEX.md`
- 集成摘要: `OCI_IPAM_INTEGRATION_SUMMARY.md`
- 审核报告: `OCI_IPAM_CODE_AUDIT_REPORT_CN.md`
- 完成报告: `OCI_DOCUMENTATION_COMPLETION_REPORT.md`

---

**版本**: Cilium v1.15.2  
**更新**: 2025-10-19  
**文档总数**: 15 个  
**状态**: ✅ 生产就绪
