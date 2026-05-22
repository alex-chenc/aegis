# Prompt 02: 前端实现

你是 Aegis 前端工程师，使用 Vue 3、Element Plus、Pinia、Axios。请实现 V5.8 动态 eBPF DetectionPackage 管理前端。

## 参考文档

- `docs/aegis_system_design_v5.8/frontend_prd_dynamic_ebpf_v5.8.md`
- `docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md`

## 需要实现

新增页面：

```text
/detection/packages
/detection/packages/new
/detection/packages/:id
/settings/ebpf-hooks
```

新增 API 封装：

```text
frontend/src/api/detection-packages.ts
frontend/src/api/ebpf-hooks.ts
```

建议组件：

```text
frontend/src/views/detection/DetectionPackages/
├── index.vue
├── PackageDetail.vue
├── PackageEditor.vue
├── components/
│   ├── PackageStatusTag.vue
│   ├── HookSummaryTable.vue
│   ├── BuildReviewPanel.vue
│   ├── HostStatusTable.vue
│   ├── CodeEditorPanel.vue
│   └── EvidenceTimeline.vue
└── composables/useDetectionPackages.ts

frontend/src/views/settings/EBPFHooks/
├── index.vue
└── components/HookAllowlistEditor.vue
```

## UI 要求

- 使用现有 Element Plus 风格，保持安全运营工具的密度和可扫描性。
- 不新增花哨视觉风格。
- 危险操作必须二次确认：签名、启用、禁用、卸载。
- 状态展示不能只靠颜色，必须有文字。
- 列表必须支持分页、筛选、搜索。
- 构建、签名、启用按钮必须有 loading 态。
- 代码编辑器区域需要显示 YAML/C 校验错误。
- Hook allowlist 中 kprobe/lsm/xdp/tc 默认空，并显示高风险提示。

## 交互流程

1. 用户创建或 AI 生成草稿。
2. 用户修改 HookPlan/eBPF/Sigma/Correlation。
3. 用户点击构建。
4. 页面轮询 build status。
5. 构建成功后展示审核信息。
6. 用户点击签名发布。
7. 用户点击启用，全局下发到全部 agent。
8. 详情页展示各主机安装状态。

## 禁止事项

- 不要自动启用签名包。
- 不要在前端构造最终 package。
- 不要在前端伪造签名状态。
- 不要隐藏 hook allowlist 不匹配风险。

## 验收

- 用户能完整完成草稿、构建、签名、启用、禁用、卸载流程。
- Hook allowlist 修改后能保存并触发后端同步。
- 主机状态表能展示 active/load_failed/blocked_by_hook_allowlist 等状态。
- 告警详情可以展示 evidence chain。

