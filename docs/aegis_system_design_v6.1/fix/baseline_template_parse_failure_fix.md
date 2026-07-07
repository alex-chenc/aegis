# 基线模板解析失败 bug 修复设计

- 版本：V6.1
- 模块：基线（Baseline）模板解析 / 前端 Workbench 基线工作区
- 日期：2026-07-07
- 作者：ZCode

## 1. Bug 描述与症状

1. **后端报错**：基线模块上传模板文件后解析报错 `解析规则失败：invalid LLM response format`。
2. **前端误显示**：解析失败后，该失败模板仍出现在前端基线工作区（"按文件查看"分组 + "全部规则"列表），用户认为"解析失败的不应该再显示"。

复现路径：
- 前端 `Workbench.vue` 上传基线文档 → `POST /api/v1/templates/upload`（或等价上传接口）→ api-server 后台任务解析。
- api-server `TemplateService.parseTemplate` 调用 LLM 提取规则 → `llm.ParseRules` 解析 LLM 响应。
- 当 LLM 返回纯文本/拒绝/错误页（不含 `[` 或 `{`）时，`ParseRules` 返回 `invalid LLM response format`，模板被标记 `status=failed`。
- 前端 `filteredTemplateGroups` 不过滤 `failed`，失败模板以空规则组 + "失败"标签显示。

## 2. 根因分析

### 2.1 后端：根因一——大文档被 MaxTokens 截断（主因）
- 错误来源链：
  - `api-server/internal/service/template_service.go` → `rules, err := llm.ParseRules(llmResponse)`
  - `api-server/internal/llm/parser.go:29` → `return nil, fmt.Errorf("invalid LLM response format")`（当 `extractJSON` 返回空串）
  - `extractJSON`（`parser.go:146`）在 JSON 数组被截断、缺少匹配的右括号（如 `]`）时返回空串。
- **主因**：`LLMClient.ChatCompletion`（`client.go:253`）硬编码 `MaxTokens: 4096`。基线文档（如 CIS 基准）规则极多，模型生成的 JSON 数组远超 4096 token，被**截断成半个数组**（缺少闭合 `]`）→ `extractJSON` 找不到匹配括号 → 报 `invalid LLM response format`。
- 日志实证（真实失败响应）：
  - `12:33:42` 响应以 `[` 开头、是合法数组，但被截断在首条规则处；
  - `12:34:34` / `12:35:10` 响应为 ` ```json\n[ ... ` 合法数组同样被截断。
  - 三次重试（含 strict 提示词）**全部**失败且表现一致——说明是确定性的 token 上限问题，而非偶发格式问题。**强化提示词重试对 token 截断无效**（这是输出长度上限，不是格式指令问题）。

### 2.2 后端：根因二——首次失败即放弃，无"强化提示词"重试（次要）
- `LLMClient.ChatCompletion`（`client.go:236`）的 retry 只对**传输/空响应**类错误重试（`isRetryableError`，`client.go:556`），而 LLM 返回"散文而非 JSON"是**一次成功的 HTTP 响应**（content 非空），不会触发重试。
- 因此：当模型偶发地在 JSON 前加了说明文字、返回了拒绝语、或返回了非 JSON 内容时，**第一次就直接判失败**，没有用更严格的提示词再试一次。强化提示词重试对"格式指令类"问题有效，但**对根因一（截断）无效**。

### 2.3 前端：失败模板未从工作区过滤
- `frontend/src/views/Workbench.vue` 的 `filteredTemplateGroups` 未排除 `status === 'failed'`（详见 2.2 原 2.2 节，未变）。

### 2.2 前端：失败模板未从工作区过滤
- `frontend/src/views/Workbench.vue`：
  - `filteredTemplateGroups`（`Workbench.vue:537`）只按 `templateFilter` 与搜索关键字过滤，**未排除 `status === 'failed'` 的模板**。
  - 派生 `allRules`（`Workbench.vue:512`）/`filteredRules`（`Workbench.vue:523`）来自各模板的 `templateRulesMap`，失败模板因无规则而无规则泄漏；但失败模板**本身**作为一个空壳分组仍被渲染（带"失败"标签 + `el-empty`）。
  - 后端 `TemplateRepository.FindAll`（`template_repo.go:35`）返回全部模板，不按 status 过滤，故失败模板直达前端。
- 影响：解析失败的"空壳"模板污染基线工作区，用户期望其不出现在可用规则视图中。

## 3. 修复设计

### 3.1 后端：增大 MaxTokens（主修复） + 文档分块（兜底）

**主修复——增大 MaxTokens：**
- `LLMClient.ChatCompletion`（及其他变体）硬编码 `MaxTokens: 4096`（约 4K token）是截断的直接原因。现代模型（gpt-4: 16K, gpt-4o: 16K, qwen-plus: 8K, deepseek: 8K）输出上限远高于此。
- 将 `client.go` 中所有 `MaxTokens: 4096` **统一增大为 131072**（128K token），远超任何单份基线文档的规则输出量（单份 CIS 基准通常 2000-6000 token），从根本上消除截断。MiniMax M2 等有独立输出上限的模型由其 `prepareRequest` 内的 cap（8192）单独保护，不受影响。
- 该改动影响所有 `ChatCompletion` 调用者（脚本生成、漏洞分析、弱密码等），均为安全/结构化分析任务，128K 输出配额对它们同样安全且有益（模型自身上限由端点自动裁剪）。

**兜底——文档分块提取：**
- 对于超大文档（输出量仍超模型实际输出上限），新增 `splitDocumentForExtraction(content)` 作为安全网：
  - 按段落（`\n\n`）切分为多个 chunk，单块字符预算 `ruleExtractChunkChars = 6000`（远小于输出 token 预算，确保每块返回完整 JSON）；超长段落做硬切分。
  - 对每个 chunk 调用新增 `extractRulesFromChunk(ctx, llmClient, chunk, idx, total)`：内部仍保留**强化提示词重试**（首次常规提示词，失败再用 `RuleExtractionPromptStrict` + temperature=0 重试最多 `ruleExtractMaxRetries=2` 次）。
  - 所有 chunk 的规则合并后由 `llm.ValidateRules` 跨块按标题去重。
  - 只要**任一** chunk 提取成功即视为整体成功；全部失败才标记 `failed`（友好提示 `解析规则失败：LLM 返回格式无法解析，请重试或更换模型`）。
- 该方案与模型输出 token 上限解耦：无论文档多大，每块都在预算内，从根本上消除"截断导致 invalid LLM response format"。
- 每次分块提取与重试均记录日志（chunk 序号、attempt），符合操作日志要求。

### 3.2 后端：增强 `extractJSON` 的健壮性（防御性）
- 在 `parser.go` 的 `extractJSON` 中，当顶层扫描找不到括号时，增加兜底：
  - 尝试匹配 ` ```json ` / ` ``` ` 代码块并提取其中内容后再解析；
  - 去除响应首尾空白/不可见字符后再扫描。
- 不改变成功路径行为，仅扩大可恢复的输入范围。

### 3.3 前端：从工作区隐藏失败模板，保留清理入口
- 在 `Workbench.vue`：
  - `filteredTemplateGroups` 中排除 `status === 'failed'` 的模板（主线"按文件查看"不再显示失败项）。
  - 派生 `allRules`/`filteredRules` 已基于 `templates`，失败模板无规则，无需额外处理；但为保险在 `allRules` 构建时同步跳过 `failed`（避免任何边界泄漏）。
  - 新增**可折叠的"解析失败的文件 (N)"区域**，列出 `status === 'failed'` 的模板及其 `删除` 按钮，保证失败模板仍可清理、不污染工作区。
- 不改动后端 API（保持返回全部模板，便于前端灵活筛选与清理）。

### 3.4 接口/数据/配置变更
- 新增 prompt 常量 `RuleExtractionPromptStrict`（`llm/prompts.go`）。
- 新增常量 `ruleExtractMaxRetries`（建议 2）于 `template_service.go`。
- 前端：纯展示层过滤 + 新增失败区，无新增接口。
- 数据库：无 schema 变更（仍使用 `templates.status` 现有 `failed` 值）。

## 4. 受影响组件
- `api-server/internal/llm/parser.go`（`extractJSON`）
- `api-server/internal/llm/prompts.go`（新增 `RuleExtractionPromptStrict`）
- `api-server/internal/service/template_service.go`（提取重试循环）
- `frontend/src/views/Workbench.vue`（`filteredTemplateGroups` / `allRules` / 失败区）

## 5. 回归测试用例
- 后端（Go，`parser_test.go` / `template_service_test.go`）：
  1. `extractJSON`：输入 ```` ```json\n[...]\n``` ```` 与含前导说明文字的 JSON 均能提取成功。
  2. `ParseRules`：输入被 markdown 包裹的 JSON 数组解析成功；空/无括号输入仍返回 `invalid LLM response format`。
  3. `splitDocumentForExtraction`：小文档聚为单块；超长文档按预算切分为多块且每块不超限；超长单段落被硬切分。
  4. `extractRulesFromChunk`：单个 chunk 在常规提示词失败、strict 提示词成功时返回规则；全失败返回 `invalid LLM response format`。（注：`template_service` 的 LLM 客户端当前为内联构造、未注入接口，故以 `splitDocumentForExtraction` 纯函数单测覆盖分块逻辑，端到端重试经构建 + 运行时验证。）
- 前端（Vitest，`Workbench.test.ts`）：
  5. `filteredTemplateGroups` 不包含 `status==='failed'` 的模板。
  6. 失败模板出现在"解析失败的文件"折叠区且可触发删除。

## 6. 风险与回滚
- 风险：分块会增加 LLM 调用次数（= chunk 数 × 每块重试次数），大文档成本与耗时上升；已有 `llmTimeout` 限制单次超时。跨块边界处个别规则可能被截断丢失（按标题去重不补偿），但远优于整文档解析失败。
- 回滚：后端改动为分块 + 每块 strict 重试 + `extractJSON` 兜底；前端改动为过滤 + 新增折叠区；均可通过 git revert 单文件回滚。

## 7. 验证步骤
- 后端：`cd api-server && go test ./internal/llm/... ./internal/service/...`
- 前端：因 `Workbench.vue` 挂载需大量 store mock，采用 `npm run build`（vite）做编译/类型校验；功能验证走运行时。
- 构建：`docker compose up -d --build api-server frontend`
- 手动：上传一份较大的基线文档（如 CIS 基准），观察不再出现 `invalid LLM response format`、规则被分块提取并合并；确认前端工作区不再显示失败模板，失败模板仅在折叠区可清理。
