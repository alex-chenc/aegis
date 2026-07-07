# 基线模块规则列表分页缺失修复设计文档

- **版本**: V6.1
- **模块**: 基线（Baseline）模块 / 前端 Workbench 规则展示
- **类型**: Bug 修复
- **关联提交**: develop（待提交）

## 1. Bug 描述与现象

在基线工作区（`Workbench.vue`）中，规则列表提供两种视图：

- **文件视角（file，默认视图）**：按基线文档（模板 / 检查规则集合）分组展示规则。
- **全部视角（all）**：跨模板扁平列表，已支持前端分页（`paginatedRules`，每页 10 条）。

问题集中在**文件视角**：每个规则集合（如上传的 `CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf`，解析出数百~上千条规则）在渲染时通过 `v-for="rule in templateRuleRows(tpl)"` **一次性把该集合下的全部规则渲染为 DOM 节点**，没有任何分页。

**现象**：

1. 当某个基线文档解析出大量规则（例如 CIS Benchmark 971 条）时，页面会渲染近千个 `<article class="file-rule-item">`，导致：
   - 首屏渲染卡顿、滚动掉帧；
   - 浏览器内存占用陡增；
   - 勾选 / 脚本状态筛选等交互卡顿。
2. 用户无法通过翻页浏览某个规则集合下的规则，只能在一个超长列表中滚动。

## 2. 复现步骤

1. 上传一个规则数量较多的基线文档（如 `CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf`）。
2. 等待解析完成（`status=completed`）。
3. 在基线工作区默认「文件视角」下观察该文档对应的分组。
4. 打开浏览器开发者工具 → Elements，观察该分组下 `<article>` 节点数量等于该文档全部规则数（数百~上千），页面无分页控件。

## 3. 根因分析

`frontend/src/views/Workbench.vue`：

- `ruleViewMode` 默认值为 `'file'`（`ruleViewMode = ref<'file' | 'all'>('file')`）。
- 文件视角模板（约 204-223 行）使用：

  ```html
  <div v-if="templateRuleRows(tpl).length" class="file-rule-list">
    <article v-for="rule in templateRuleRows(tpl)" :key="rule.id" class="file-rule-item">
  ```

  `templateRuleRows(tpl)` 直接返回该模板在 `templateRulesMap[tpl.id]` 中的**全量**规则数组（仅做关键字过滤），未被切片。

- 对比「全部视角」已通过 `paginatedRules` + `<el-pagination>` 做了客户端分页（每页 10 条），文件视角缺少等价的分页逻辑与控件。

**结论**：文件视角缺少与「全部视角」一致的客户端分页（每页 10 条）实现与分页控件。

> 说明：后端 `GET /api/v1/templates/:id/rules`（`TemplateHandler.GetTemplateRules` → `RuleRepository.FindByTemplateID`）当前一次性返回该模板全部规则，与「全部视角」既有客户端分页模式一致（数据已在内存中，按页切片展示）。本次修复沿用既有客户端分页模式，**不改动后端接口契约**，以保持跨模板搜索、规则勾选、任务下发等依赖全量内存数据的逻辑不受影响，属于最小安全修复。后端分页作为后续增强项在文档「风险与回滚」中记录。

## 4. 修复设计

### 4.1 目标

- 文件视角下，**每个规则集合分组独立分页**，每页 10 条规则。
- 分页状态（当前页）按模板 ID 隔离，切换模板不影响彼此。
- 关键字搜索 / 模板筛选变化时，重置各分组的当前页到第一页。
- 规则总数徽标、勾选状态、脚本操作等既有行为保持不变。

### 4.2 组件改造（`frontend/src/views/Workbench.vue`）

1. **新增每页大小常量**：`const fileRulePageSize = 10`（与 `rulePageSize` 保持一致）。

2. **新增按模板隔离的当前页码映射**：

   ```ts
   const templateRulePageMap = reactive<Record<string, number>>({})
   function templateRuleCurrentPage(tplId: string): number {
     return templateRulePageMap[tplId] || 1
   }
   function setTemplateRulePage(tplId: string, page: number) {
     templateRulePageMap[tplId] = page
   }
   ```

3. **新增分页切片计算**（复用 `templateRuleRows` 的全量过滤结果，仅切片）：

   ```ts
   function templateRulePageRows(tpl: Template): RuleRow[] {
     const rows = templateRuleRows(tpl)
     const start = (templateRuleCurrentPage(tpl.id) - 1) * fileRulePageSize
     return rows.slice(start, start + fileRulePageSize)
   }
   ```

4. **模板渲染调整**：
   - 将 `v-for="rule in templateRuleRows(tpl)"` 改为 `v-for="rule in templateRulePageRows(tpl)"`。
   - 在 `.file-rule-list` 之后新增 `<el-pagination>`，绑定按模板的当前页、总条数、每页大小：

     ```html
     <el-pagination
       v-if="templateRuleRows(tpl).length > fileRulePageSize"
       :current-page="templateRuleCurrentPage(tpl.id)"
       :page-size="fileRulePageSize"
       :total="templateRuleRows(tpl).length"
       layout="total, prev, pager, next"
       background
       @current-change="page => setTemplateRulePage(tpl.id, page)"
     />
     ```

5. **搜索 / 筛选变化时重置页码**（watch）：

   ```ts
   watch([ruleSearch, templateFilter], () => {
     for (const key of Object.keys(templateRulePageMap)) delete templateRulePageMap[key]
   })
   ```

### 4.3 可测试纯函数（新增 `frontend/src/utils/paginate.ts`）

为便于回归测试，提取通用分页切片函数：

```ts
export function paginate<T>(items: T[], page: number, pageSize: number): T[] {
  if (pageSize <= 0) return items
  const p = page < 1 ? 1 : page
  const start = (p - 1) * pageSize
  return items.slice(start, start + pageSize)
}
```

并在 `templateRulePageRows` 中调用 `paginate(rows, templateRuleCurrentPage(tpl.id), fileRulePageSize)`，保持「全部视角」的 `paginatedRules` 也可后续复用该函数（本次不强制改动，降低风险）。

## 5. 代码变更清单

| 文件 | 变更 |
| --- | --- |
| `frontend/src/utils/paginate.ts` | 新增 `paginate<T>()` 纯函数 |
| `frontend/src/utils/paginate.test.ts` | 新增单元测试（边界、空数组、末页、越界页） |
| `frontend/src/views/Workbench.vue` | 新增 `fileRulePageSize`、按模板页码映射、`templateRulePageRows`、`el-pagination` 控件、watch 重置 |
| `docs/aegis_system_design_v6.1/fix/baseline_rule_list_pagination_fix.md` | 本设计文档 |

> 后端接口、proto、模型、仓库层均**不改动**。

## 6. 验证步骤

1. **单元测试**：`cd frontend && npm run test -- src/utils/paginate.test.ts`（全部通过）。
2. **前端类型检查**：`cd frontend && npm run type-check`（无错误）。
3. **前端构建**：`cd frontend && npm run build`（成功）。
4. **集成（aegis-build-test）**：
   - 重新构建并启动前端容器。
   - 基线工作区默认文件视角下，上传/加载一个多规则文档，确认每个集合分组底部出现分页控件，每页 10 条，翻页正常，勾选与脚本操作不受影响。
   - 输入关键字搜索后，分页回到第 1 页。

## 7. 影响范围

- 仅影响前端基线工作区文件视角的规则渲染方式（展示层）。
- 不改变：
  - 后端接口契约与数据结构；
  - 「全部视角」既有分页行为；
  - 规则勾选状态、任务下发、脚本编辑等逻辑（依赖全量内存数据，保持不变）。

## 8. 风险与回滚

- **风险**：低。纯前端展示层分页，复用了「全部视角」已验证的客户端分页模式。
- **回滚**：删除 `frontend/src/utils/paginate.ts` 与 `Workbench.vue` 中的分页相关改动（约 20 行），恢复 `v-for` 直接遍历 `templateRuleRows(tpl)` 即可。
- **后续增强（非本次）**：如需进一步降低大数据量下的前端内存与网络负载，可在后端 `GET /templates/:id/rules` 增加 `page`/`page_size` 查询参数与总数返回，前端按模板懒加载分页。该方案需要同步改造跨模板搜索、勾选、下发逻辑（依赖全量内存数据），故不在本次最小安全修复范围内。

## 9. 补充：规则按脚本就绪度排序

在分页修复基础上，进一步要求规则列表按脚本就绪度排序：**已生成 > 生成中 > 未生成/失败**。

- 新增 `frontend/src/utils/ruleSort.ts`：`ruleScriptStatusRank(rule)` 与 `compareRulesByScriptStatus(a, b)`。
  - 综合检测 / 修复脚本状态：任一为 `generated` → 0（最前）；否则任一为 `generating` → 1（中间）；其余（pending / failed）→ 2（最后）。
- `Workbench.vue` 中 `allRules`（「全部视角」与搜索/下发数据源）与 `templateRuleRows`（「文件视角」）均在映射+过滤后 `.sort(compareRulesByScriptStatus)`，因此排序对两种视图、分页、任务下发选择一致生效。
- 新增 `frontend/src/utils/ruleSort.test.ts`（5 用例）覆盖分级与排序顺序，全部通过。

