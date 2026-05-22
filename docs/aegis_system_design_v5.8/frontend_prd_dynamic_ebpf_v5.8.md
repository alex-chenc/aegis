# V5.8 前端 PRD: 动态 eBPF DetectionPackage 管理

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. 产品目标

为安全团队提供一套可运营的动态 eBPF 检测包管理界面，使管理员可以完成：

- 查看 AI 生成的检测草稿。
- 修改 HookPlan、Sigma atomic rules、Correlation DetectionSpec 和 eBPF 源码。
- 提交构建并查看构建结果。
- 对构建产物进行人工签名发布。
- 配置全局 hook allowlist。
- 启用/禁用/卸载 DetectionPackage。
- 查看全部 agent 的安装和运行状态。
- 查看 correlation alert 的 evidence chain。

---

## 2. 导航结构

新增菜单：

```text
智能异常检测
├── 安全概览
├── 告警中心
├── 阻断策略
├── 规则管理
└── 动态检测包 (V5.8新增)

系统配置
├── 模型配置
├── 命令审计配置
├── 审计日志
├── eBPF Hook 白名单 (V5.8新增)
└── Agent 安装
```

路由：

| 页面 | 路由 | 说明 |
|:---|:---|:---|
| 动态检测包列表 | `/detection/packages` | package 生命周期管理 |
| 新建检测草稿 | `/detection/packages/new` | AI 生成或手动创建草稿 |
| 检测包详情 | `/detection/packages/:id` | 构建、签名、启用、状态 |
| Hook 白名单 | `/settings/ebpf-hooks` | 全局 hook allowlist 配置 |

---

## 3. 用户角色

| 角色 | 权限 |
|:---|:---|
| 安全分析师 | 创建 AI 草稿、查看构建结果、查看状态 |
| 安全开发大师 | 修改 eBPF 源码、HookPlan、规则 |
| 管理员 | 修改 hook allowlist、签名发布、启用/禁用/卸载 package |

第一版可复用现有登录体系，不新增 RBAC 表；页面操作按钮按后端权限能力预留禁用态。

---

## 4. 动态检测包列表页

### 4.1 信息架构

列表字段：

| 字段 | 说明 |
|:---|:---|
| Package ID | 稳定身份，如 `cve-2026-31431-copyfail` |
| 标题 | 检测包名称 |
| 版本 | SemVer |
| 状态 | draft/built/signed/enabled/active/disabled |
| CVE | 关联 CVE |
| Hook 数量 | manifest 中 hook 数量 |
| 安装成功率 | active agent 数 / total agent 数 |
| 最近构建 | 构建时间和结果 |
| 操作 | 详情、启用、禁用、卸载 |

筛选：

- 状态
- CVE
- 严重等级
- 是否启用
- 构建结果

搜索：

- package_id
- title
- CVE

### 4.2 状态展示

状态标签：

| 状态 | 颜色 |
|:---|:---|
| draft | gray |
| build_failed | red |
| built | blue |
| signed | cyan |
| enabled | green |
| active | green |
| degraded | orange |
| disabled | gray |

UI 要求：

- 表格密度适中，适合安全运营工具。
- 状态颜色必须同时配文字，不只靠颜色表达。
- 操作按钮带确认弹窗，尤其是启用、禁用、卸载。

---

## 5. 新建/编辑草稿页

### 5.1 创建方式

提供两种入口：

1. **AI 生成草稿**
   - 输入 CVE 编号、漏洞描述、参考事件、目标内核范围。
   - AI 生成 HookPlan、Sigma、Correlation、源码草稿。

2. **手动创建**
   - 管理员手动填写 package_id、version、HookPlan 等。

### 5.2 编辑区域

页面采用多标签页：

```text
基础信息 | HookPlan | eBPF 源码 | Sigma 原子规则 | Correlation | 构建参数
```

各标签页要求：

- 使用代码编辑器组件，支持 YAML/C 语法高亮。
- 保存时只保留最后一个草稿版本，不做 revision 历史。
- 支持基础 YAML 格式校验。
- HookPlan 和 manifest 不一致时提示。
- Correlation 引用的 rule_id 必须来自当前 package 的 Sigma。

### 5.3 保存规则

- `package_id` 创建后不可编辑。
- `version` 可编辑，但必须 SemVer。
- 保存草稿不触发构建。
- 构建必须人工点击。

---

## 6. 构建审核页

构建成功后展示人工审核信息：

| 区块 | 内容 |
|:---|:---|
| 基础信息 | package_id、version、title、CVE、风险等级 |
| 构建环境 | builder image、digest、clang 版本、构建时间 |
| Hook 列表 | attach_type、attach、program、是否在 allowlist |
| Artifact | perf/ringbuf 文件名、大小 |
| Event Schema | event_type、字段 ID、字段名、类型 |
| Sigma | atomic rule 列表和 YAML |
| Correlation | sequence、window、by |
| 限速 | per plugin、per event_type、per pid |
| 构建日志 | 摘要和完整日志查看 |

按钮：

- `重新构建`
- `签名发布`
- `返回编辑`

规则：

- 构建失败不允许签名。
- hook 不在全局 allowlist 时允许签名，但启用时会提示并可能被 agent 拒绝。
- 签名发布必须二次确认。

---

## 7. 启用确认

管理员点击启用时，弹窗必须展示：

```text
该 DetectionPackage 将下发到全部 agent。
离线 agent 上线后也会收到安装指令。
```

确认信息：

- package_id/version
- hooks
- artifact 大小
- 当前 allowlist 校验结果
- 风险提示

启用后：

- api-server 创建全局启用记录。
- server 向在线 agent 下发 install command。
- 页面进入主机状态轮询。

---

## 8. Hook 白名单配置页

路由：`/settings/ebpf-hooks`

### 8.1 默认配置

页面首次初始化默认白名单：

```yaml
tracepoints:
  - syscalls/sys_enter_socket
  - syscalls/sys_enter_bind
  - syscalls/sys_enter_splice
  - syscalls/sys_enter_execve
  - syscalls/sys_exit_execve
  - syscalls/sys_enter_setuid
  - syscalls/sys_enter_setgid
  - syscalls/sys_enter_capset
  - sched/sched_process_fork
  - sched/sched_process_exit
kprobes: []
lsm: []
xdp: []
tc: []
```

### 8.2 编辑体验

页面组件：

- 分类表格：tracepoints/kprobes/lsm/xdp/tc。
- 新增 hook 输入框。
- 批量导入 YAML。
- 保存前校验重复项和非法格式。
- 风险提示：kprobe/lsm/xdp/tc 标记为高风险。

保存后：

- api-server 保存全局 allowlist。
- server 广播 `dynamic_ebpf_hook_allowlist` config 到所有在线 agent。
- agent 对已安装 package 重新校验，不合规则停用并上报。

---

## 9. 主机级状态页

在 package 详情页提供主机状态表：

| 字段 | 说明 |
|:---|:---|
| host_id | 主机 ID |
| hostname | 主机名 |
| kernel | 内核版本 |
| arch | 架构 |
| status | pending/downloading/active/load_failed 等 |
| active_artifact | ringbuf/perf |
| loaded_hooks | 已加载 hook |
| error_message | 失败原因 |
| last_reported_at | 最近上报时间 |

筛选：

- status
- active_artifact
- error reason

状态统计卡片：

- 总主机
- active
- degraded
- failed
- blocked by allowlist

---

## 10. 告警详情 evidence 展示

Correlation 告警详情页新增 evidence chain：

```text
Evidence Chain
1. AF_ALG socket created
2. AF_ALG bind to AEAD
3. splice called
4. suspicious root exec
```

每项 evidence 展示：

- timestamp
- rule_id
- pid/ppid/uid
- process_name/commandline
- key fields
- process tree

UI 要求：

- 使用纵向时间线。
- 每个 evidence 可展开查看 `event_data_json`。
- 不展示未命中的所有原始插件事件。

---

## 11. 前端 API 封装

建议新增：

```text
frontend/src/api/detection-packages.ts
frontend/src/api/ebpf-hooks.ts
```

接口草案：

```typescript
export const detectionPackageApi = {
  list: (params) => request.get('/api/v1/detection/packages', { params }),
  get: (id) => request.get(`/api/v1/detection/packages/${id}`),
  createDraft: (data) => request.post('/api/v1/detection/packages/drafts', data),
  updateDraft: (id, data) => request.put(`/api/v1/detection/packages/drafts/${id}`, data),
  generateDraft: (data) => request.post('/api/v1/detection/packages/ai-generate', data),
  build: (id) => request.post(`/api/v1/detection/packages/${id}/build`),
  sign: (id) => request.post(`/api/v1/detection/packages/${id}/sign`),
  enable: (id) => request.post(`/api/v1/detection/packages/${id}/enable`),
  disable: (id) => request.post(`/api/v1/detection/packages/${id}/disable`),
  uninstall: (id) => request.post(`/api/v1/detection/packages/${id}/uninstall`),
  hostStatus: (id, params) => request.get(`/api/v1/detection/packages/${id}/hosts`, { params }),
}

export const ebpfHookApi = {
  getAllowlist: () => request.get('/api/v1/settings/ebpf-hooks/allowlist'),
  updateAllowlist: (data) => request.put('/api/v1/settings/ebpf-hooks/allowlist', data),
}
```

---

## 12. 可用性与可访问性要求

- 所有危险操作必须确认。
- 构建、签名、启用按钮必须有 loading 态。
- 表格支持分页，状态刷新避免频繁抖动。
- 代码编辑器错误提示必须靠近对应 tab。
- 颜色不作为唯一状态表达，必须有状态文本。
- 关键按钮保持可键盘操作。
- 默认采用现有 Element Plus 设计系统，不引入新 UI 框架。

