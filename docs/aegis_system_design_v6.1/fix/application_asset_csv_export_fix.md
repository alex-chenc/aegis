# 应用资产 CSV 导出失效修复设计文档

- **版本**: V6.1
- **模块**: 主机资产 / 应用资产（`frontend/src/views/hosts/Assets/Applications.vue`）
- **类型**: Bug 修复
- **关联提交**: develop（待提交）

## 1. Bug 描述与现象

应用资产页面右上角「导出 CSV」按钮点击后**没有任何反应**：既不生成文件，也无报错提示。用户无法将应用资产导出为 CSV。

## 2. 复现步骤

1. 进入「主机资产 → 应用资产」（或某个分类页，如数据库 / Web 服务）。
2. 确认列表已有数据（或先执行查询）。
3. 点击右上角「导出 CSV」按钮。
4. 观察：无文件下载、无提示、浏览器控制台无报错。

## 3. 根因分析

`frontend/src/views/hosts/Assets/Applications.vue` 中 `handleExport()` 函数体为空，仅留 `// TODO: 实现 CSV 导出` 占位：

```ts
// 导出 CSV
function handleExport() {
  // TODO: 实现 CSV 导出
}
```

按钮 `click` 绑定到该函数，但函数未实现任何逻辑，因此点击后静默无效果。

> 同文件 `Software.vue`（软件资产）也存在完全相同的 `// TODO: 实现 CSV 导出` 空实现，属于同类缺陷；本次仅修复用户报告的「应用资产」导出，「软件资产」导出留作后续（见风险与回滚）。

## 4. 修复设计

### 4.1 目标

- 点击「导出 CSV」后，导出**当前筛选条件下的全部应用资产**（不受表格当前分页限制），生成 UTF-8（带 BOM）CSV 文件并触发浏览器下载。
- 字段需正确转义（逗号、引号、换行），避免中文乱码与字段错位。
- 空结果时给出友好提示；导出中按钮显示 loading，避免重复点击。

### 4.2 通用 CSV 工具（新增 `frontend/src/utils/csv.ts`）

- `escapeCsvField(value)`：RFC 4180 转义——含逗号/引号/换行的字段加双引号并翻倍内部引号。
- `buildCsv(headers, rows)`：以 CRLF 连接表头与数据行。
- `downloadCsv(filename, csv)`：用 `Blob` 生成带 UTF-8 BOM（`﻿`）的内容并创建 `<a download>` 触发下载，结束后回收 URL。

### 4.3 组件改造（`Applications.vue`）

- `handleExport()` 实现：
  1. 基于当前 `filters`（关键字 / 分类 / 复核状态等）请求 `listApplicationAssets`，将 `page_size` 设为 `max(applicationTotal, 当前页条数, 1)`，拉取全部匹配数据。
  2. 无数据时 `ElMessage.warning('当前筛选条件下没有可导出的应用资产')` 并返回。
  3. 按表格列映射 16 个字段（主机名称、主机ID、IP、分组、操作系统、应用名称、分类、标签、版本、PID、监听端口、启动用户、启动路径、置信度、状态、记录时间），复用既有格式化函数（`getCategoryLabel` / `applicationRuntimeLabel` / `getConfidence` / `displayPids` / `getReviewStatusLabel` / `formatTime`）。
  4. `buildCsv` 生成内容，`downloadCsv` 下载，文件名形如 `应用资产_20260708_053000.csv`。
  5. 成功提示导出条数；异常时 `ElMessage.error('导出 CSV 失败')`。
- 新增 `exporting` ref，按钮绑定 `:loading="exporting"`，函数开头 `if (exporting.value) return` 防止并发。
- 新增导入：`listApplicationAssets`（来自 `@/api/assets`）、`buildCsv` / `downloadCsv`（来自 `@/utils/csv`）。

## 5. 代码变更清单

| 文件 | 变更 |
| --- | --- |
| `frontend/src/utils/csv.ts` | 新增 `escapeCsvField` / `buildCsv` / `downloadCsv` |
| `frontend/src/utils/csv.test.ts` | 新增单测（转义、拼接、下载触发，8 用例） |
| `frontend/src/views/hosts/Assets/Applications.vue` | 实现 `handleExport`，新增 `exporting` 与导入，按钮 loading |

## 6. 验证步骤

1. **单元测试**：`cd frontend && npx vitest run src/utils/csv.test.ts`（8/8 通过）。
2. **构建**：`cd frontend && npm run build`（成功）。
3. **集成（aegis-build-test）**：
   - `docker compose up -d --build frontend` 重启后 `curl http://localhost:8081/` 返回 200。
   - 浏览器进入「应用资产」，点击「导出 CSV」，确认下载 `应用资产_*.csv`，用 Excel/WPS 打开中文正常、字段对齐；含逗号/换行的字段被正确包裹。
   - 切换分类 / 输入关键字后导出，确认仅导出匹配数据。
   - 空结果（如不匹配的关键字）时提示「没有可导出的应用资产」。

## 7. 影响范围

- 仅影响应用资产页面的 CSV 导出功能（纯前端）。
- 不改变后端接口契约；复用既有 `GET /host-assets/applications` 列表接口。
- 「软件资产」导出仍为 TODO（未改动，见下）。

## 8. 风险与回滚

- **风险**：低。纯前端导出逻辑，复用既有接口与格式化函数；CSV 转义符合 RFC 4180，并加 UTF-8 BOM 兼容 Excel。导出全量数据对超大资产量会有一次较大查询，但应用资产量级通常可控。
- **回滚**：删除 `frontend/src/utils/csv.ts`、测试文件，并将 `handleExport` 还原为 `// TODO: 实现 CSV 导出` 即可。
- **后续增强**：
  - `Software.vue` 的「导出 CSV」为同样的空 TODO，可套用本 `csv.ts` 一并实现。
  - 若需限制导出规模，可在后端增加专门的导出接口或最大条数上限。
