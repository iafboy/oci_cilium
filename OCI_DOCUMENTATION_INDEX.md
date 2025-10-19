# OCI IPAM 文档索引

本目录包含 Cilium v1.15.2 OCI IPAM 集成的完整文档。

**最后更新**: 2025年10月19日

---

## 📁 文档结构

### 根目录文档 (项目根目录)

| 文件名 | 大小 | 说明 | 语言 |
|--------|------|------|------|
| `OCI_IPAM_INTEGRATION_SUMMARY.md` | 19KB | 集成摘要文档，包含完整的架构、代码清单、配置示例 | 🇬🇧 英文 |
| `OCI_IPAM_CODE_AUDIT_REPORT_CN.md` | 20KB | 完整的代码审核报告，包含所有修改文件的详细分析 | 🇨🇳 中文 |
| `OCI_IPAM_README_CN.md` | 18KB | 快速入门和概览文档，面向普通用户 | 🇨🇳 中文 |


### 用户文档 (Documentation/network/oci/)

| 文件名 | 大小 | 说明 | 语言 |
|--------|------|------|------|
| `README.md` | 9.3KB | OCI IPAM 总览、架构、特性介绍 | 🇬🇧 英文 |
| `README_CN.md` | 9.0KB | OCI IPAM 总览、架构、特性介绍 | 🇨🇳 中文 |
| `quickstart.md` | 11KB | 5步快速入门指南，包含完整部署流程 | 🇬🇧 英文 |
| `quickstart_CN.md` | 12KB | 5步快速入门指南，包含完整部署流程 | 🇨🇳 中文 |
| `configuration.md` | 16KB | 完整配置参考，包含所有参数说明 | 🇬🇧 英文 |
| `configuration_CN.md` | 4.2KB | 完整配置参考，包含所有参数说明 | 🇨🇳 中文 |
| `troubleshooting.md` | 18KB | 30+ 故障场景的诊断和解决方案 | 🇬🇧 英文 |
| `troubleshooting_CN.md` | 21KB | 30+ 故障场景的诊断和解决方案 | 🇨🇳 中文 |

---

## 📖 文档使用指南

### 🚀 新用户快速开始

**如果您是第一次使用 OCI IPAM，建议按以下顺序阅读**:

1. **了解概念** → `OCI_IPAM_README_CN.md` 或 `Documentation/network/oci/README_CN.md`
   - 理解什么是 OCI IPAM
   - 查看工作原理和架构
   - 了解核心特性

2. **快速部署** → `Documentation/network/oci/quickstart_CN.md`
   - 按照 5 步指南部署
   - 验证安装
   - 测试 Pod 网络

3. **调整配置** → `Documentation/network/oci/configuration_CN.md`
   - 查看所有配置参数
   - 根据需求调整

4. **遇到问题?** → `Documentation/network/oci/troubleshooting_CN.md`
   - 诊断常见错误
   - 查找解决方案
   - 高级调试技巧

### 🔧 开发者深度了解

**如果您想理解代码实现或进行二次开发**:

1. **集成概览** → `OCI_IPAM_INTEGRATION_SUMMARY.md`
   - 完整的文件清单（38 个新文件 + 15 个修改）
   - 架构设计详解
   - Build tags 和依赖项
   - 集成点分析

2. **代码审核** → `OCI_IPAM_CODE_AUDIT_REPORT_CN.md`
   - 每个文件的详细分析
   - 代码质量评估
   - 已修复的问题
   - 风险评估

3. **生成文件说明** → `OCI_GENERATED_FILES_README.md`
   - 理解 `zz_generated.*.go` 文件
   - 学习如何重新生成
   - 最佳实践

---

## 📚 按场景查找文档

### 场景 1: 我想快速部署 OCI IPAM

📖 **推荐文档**: `Documentation/network/oci/quickstart_CN.md`

**包含内容**:
- ✅ 前置条件检查
- ✅ IAM 策略配置
- ✅ Helm values 示例
- ✅ 安装命令
- ✅ 验证步骤

---

### 场景 2: Pod 无法获取 IP 地址

📖 **推荐文档**: `Documentation/network/oci/troubleshooting_CN.md`

**查找章节**:
- "IPAM 问题" → "Pod 卡在 ContainerCreating"
- "常见错误" → "failed to allocate IP"
- "诊断工具" → 使用诊断命令

---

### 场景 3: 我想调整 VNIC 预分配策略

📖 **推荐文档**: `Documentation/network/oci/configuration_CN.md`

**查找参数**:
- `oci.vnicPreAllocationThreshold`
- `oci.maxIPsPerVNIC`
- `oci.maxVNICsPerNode`

---

### 场景 4: 我想了解代码架构

📖 **推荐文档**: `OCI_IPAM_INTEGRATION_SUMMARY.md`

**查找章节**:
- "二、代码集成详情" → 完整文件清单
- "五、集成点分析" → IPAM 框架、CRD、Operator 集成
- "架构设计" → 组件架构图

---

### 场景 5: 性能优化

📖 **推荐文档**: 
- `Documentation/network/oci/README_CN.md` → "性能考虑"
- `OCI_IPAM_INTEGRATION_SUMMARY.md` → "十二、性能特性"
- `Documentation/network/oci/troubleshooting_CN.md` → "性能问题"

---

### 场景 6: 权限配置

📖 **推荐文档**: `Documentation/network/oci/quickstart_CN.md`

**查找章节**:
- "第 1 步: 准备 OCI 环境" → 1.3 设置 IAM 策略

---

---

## 🌐 语言版本对照

| 内容 | 英文版 | 中文版 |
|------|--------|--------|
| OCI IPAM 总览 | `Documentation/network/oci/README.md` | `Documentation/network/oci/README_CN.md` |
| 快速入门 | `Documentation/network/oci/quickstart.md` | `Documentation/network/oci/quickstart_CN.md` |
| 故障排查 | `Documentation/network/oci/troubleshooting.md` | `Documentation/network/oci/troubleshooting_CN.md` |
| 配置参考 | `Documentation/network/oci/configuration.md` | `Documentation/network/oci/configuration_CN.md` |
| 代码审查 | `OCI_IPAM_CODE_AUDIT_REPORT_CN.md` |
| 快速入门概览 | - | `OCI_IPAM_README_CN.md` |

---

## 📊 文档统计

### 总体统计

| 指标 | 数量 |
|------|------|
| 文档总数 | 13 个 |
| 总字数 | ~50,000 字 |
| 总大小 | ~170KB |
| 中文文档 | 6 个 |
| 英文文档 | 7 个 |

### 按类型统计

| 类型 | 数量 | 文档 |
|------|------|------|
| 用户指南 | 8 | README, quickstart, configuration, troubleshooting (中英文各 4) |
| 开发者文档 | 3 | 集成摘要、审核报告、代码审查 |
| 技术说明 | 2 | 生成文件说明、快速入门概览 |

---

## 🔄 文档更新历史

| 日期 | 版本 | 变更 |
|------|------|------|
| 2025-10-19 | 1.0 | 初始版本，包含完整的中英文文档 |

---

## ✅ 文档完整性检查清单

### 用户文档
- [x] 英文总览 (README.md)
- [x] 中文总览 (README_CN.md)
- [x] 英文快速入门 (quickstart.md)
- [x] 中文快速入门 (quickstart_CN.md)
- [x] 英文配置参考 (configuration.md)
- [x] 中文配置参考 (configuration_CN.md)
- [x] 英文故障排查 (troubleshooting.md)
- [x] 中文故障排查 (troubleshooting_CN.md)

### 开发者文档
- [x] 集成摘要 (OCI_IPAM_INTEGRATION_SUMMARY.md)
- [x] 完整审核报告 (OCI_IPAM_CODE_AUDIT_REPORT_CN.md)
- [x] 快速入门概览 (OCI_IPAM_README_CN.md)


### 特殊文档
- [x] 文档索引 (本文件)

---

## 📝 文档贡献

如需更新文档，请遵循以下规范:

### 中文文档规范
- 使用简体中文
- 专业术语首次出现时提供英文对照
- 代码和命令使用英文
- 保持与英文文档内容同步

### 英文文档规范
- 使用美式英语拼写
- 技术术语使用行业标准表达
- 简洁明了，避免冗长

### Markdown 规范
- 使用标准 Markdown 语法
- 代码块指定语言类型
- 表格对齐整齐
- 使用 emoji 增强可读性（适度）

---

## 🔗 相关链接

- **Cilium 官方文档**: https://docs.cilium.io/
- **OCI SDK Go 文档**: https://docs.oracle.com/en-us/iaas/tools/go/latest/
- **Kubernetes IPAM**: https://kubernetes.io/docs/concepts/cluster-administration/networking/
- **GitHub 仓库**: https://github.com/cilium/cilium

---

## 📧 反馈

如有文档问题或建议:
- 提交 GitHub Issue
- 联系维护团队
- 通过 Cilium Slack 讨论

---

**维护者**: SEHUB 
**文档版本**: 1.0  
**最后更新**: 2025年10月19日
