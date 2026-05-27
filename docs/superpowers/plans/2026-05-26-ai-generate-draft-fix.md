# AI生成检测包草稿功能修复 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复AI生成检测包草稿功能的3个关键bug，使LLM被正确调用并返回真实内容

**Architecture:** 最小修复方案 — 修复超时单位bug、LLM失败时返回错误而非静默降级、传递缺失的attack_prerequisites字段给LLM prompt

**Tech Stack:** Go (api-server), TypeScript/Vue 3 (frontend), LLM API

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `api-server/internal/api/handler/detection_package_handler.go` | Modify | 修复超时bug、错误处理、字段传递 |
| `api-server/internal/llm/prompts.go` | Modify | 增加attack_prerequisites参数到prompt模板 |
| `frontend/src/views/detection/DetectionPackages/PackageEditor.vue` | Modify | 增强AI生成失败的错误反馈 |
| `docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md` | Modify | 更新AI生成API文档 |

---

### Task 1: 修复后端超时bug和LLM失败错误处理

**Files:**
- Modify: `api-server/internal/api/handler/detection_package_handler.go:78-141`

- [ ] **Step 1: 修复超时单位和LLM失败处理**

将 `AIGenerateDraft` 方法中的超时从 `120`（纳秒）改为 `120*time.Second`，并将LLM失败时的静默降级改为返回503错误。同时将 `attack_prerequisites` 传入LLM prompt。

替换 `detection_package_handler.go` 第96-117行：

```go
	// Call LLM to generate detection package content
	hookPlanYAML := ""
	ebpfSource := ""
	sigmaRulesYAML := ""
	correlationYAML := ""

	if h.llmClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "AI生成服务未配置，请检查LLM配置"})
		return
	}

	prompt := llm.GetDetectionPackageGenerationPrompt(
		req.CVEID,
		req.VulnerabilityDescription,
		req.AttackPrerequisites,
		req.ExploitationChain,
		req.FalsePositiveConstraints,
	)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	response, err := h.llmClient.ChatCompletion(ctx, "", prompt, 0.7)
	if err != nil {
		logger.Error("LLM generation failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    503,
			"message": fmt.Sprintf("AI生成失败: %s，请检查LLM配置或稍后重试", err.Error()),
		})
		return
	}

	hookPlanYAML, ebpfSource, sigmaRulesYAML, correlationYAML = parseLLMResponse(response)
```

- [ ] **Step 2: 验证编译通过**

Run: `cd /code/ai-benchmark/api-server && go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 3: Commit**

```bash
git add api-server/internal/api/handler/detection_package_handler.go
git commit -m "fix: AI生成草稿超时单位bug和LLM失败错误处理"
```

---

### Task 2: 更新LLM prompt模板增加attack_prerequisites参数

**Files:**
- Modify: `api-server/internal/llm/prompts.go:393-469`

- [ ] **Step 1: 更新DetectionPackageGenerationPrompt模板和函数签名**

替换 `prompts.go` 第393-469行：

```go
// DetectionPackageGenerationPrompt generates detection package drafts from CVE information
const DetectionPackageGenerationPrompt = `你是 Aegis V5.8 的 AI 安全规则生成器。你的输出是人工可修改的草稿，不是最终发布物。

## 输入

你会收到：
- CVE 编号
- 漏洞描述
- 攻击前置条件
- 利用链行为
- 可观测系统调用或内核 hook
- 误报约束
- 当前 agent 支持能力

## 输出

必须输出四段，每段使用对应语言标记的代码块：

1. HookPlan YAML - 只描述 hook、extract、filter、emit，不包含告警逻辑
2. eBPF C 源码草稿 - 只做事件采集和轻量过滤，不做复杂检测
3. Sigma atomic rules YAML - 只做单事件 atomic detection
4. Correlation DetectionSpec YAML - 只做 ordered sequence + window + by

## 关键规则

- HookPlan 只描述采集，不描述告警。
- eBPF 插件只做事件采集和轻量过滤。
- Sigma 只做单事件 atomic detection。
- Correlation 只做 ordered sequence + window + by。
- rule_id 使用 package_id.stable_name 格式。
- 不生成跨 package 依赖。
- 不使用未明确允许的 hook 类型（默认只允许 tracepoint）。
- 输出必须避免不可控事件风暴。

## 输出模板

请按以下章节输出：

## Package Metadata
package_id, version, title, description, cve_ids

## HookPlan
使用 yaml 代码块

## eBPF Source Draft
使用 c 代码块

## Sigma Atomic Rules
使用 yaml 代码块

## Correlation DetectionSpec
使用 yaml 代码块

## 风险与限制
说明检测的边界和潜在误报

## 安全边界声明

请在输出末尾明确写出：
该输出为草稿，必须经过人工修改、builder 容器编译、人工审核、人工签名发布和页面启用后，才能由 agent 安装。

CVE 信息：
%s

漏洞描述：
%s

攻击前置条件：
%s

利用链行为：
%s

误报约束：
%s`

// GetDetectionPackageGenerationPrompt returns the detection package generation prompt
func GetDetectionPackageGenerationPrompt(cveID, description, prerequisites, chain, constraints string) string {
	return fmt.Sprintf(DetectionPackageGenerationPrompt, cveID, description, prerequisites, chain, constraints)
}
```

- [ ] **Step 2: 验证编译通过**

Run: `cd /code/ai-benchmark/api-server && go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 3: Commit**

```bash
git add api-server/internal/llm/prompts.go
git commit -m "fix: LLM prompt增加attack_prerequisites参数"
```

---

### Task 3: 增强LLM响应解析器

**Files:**
- Modify: `api-server/internal/api/handler/detection_package_handler.go:143-199`

- [ ] **Step 1: 重写parseLLMResponse和extractCodeBlock函数**

替换 `detection_package_handler.go` 第143-199行：

```go
// parseLLMResponse extracts the four sections from the LLM response
func parseLLMResponse(response string) (hookPlan, ebpfSource, sigmaRules, correlation string) {
	// Strategy 1: Try section-hint based extraction
	hookPlan = extractCodeBlock(response, "yaml", "HookPlan")
	ebpfSource = extractCodeBlock(response, "c", "eBPF Source")
	sigmaRules = extractCodeBlock(response, "yaml", "Sigma")
	correlation = extractCodeBlock(response, "yaml", "Correlation")

	// Strategy 2: If section-hint extraction failed, try positional fallback
	if hookPlan == "" || ebpfSource == "" || sigmaRules == "" || correlation == "" {
		hookPlan2, ebpfSource2, sigmaRules2, correlation2 := extractCodeBlocksByPosition(response)
		if hookPlan == "" {
			hookPlan = hookPlan2
		}
		if ebpfSource == "" {
			ebpfSource = ebpfSource2
		}
		if sigmaRules == "" {
			sigmaRules = sigmaRules2
		}
		if correlation == "" {
			correlation = correlation2
		}
	}

	// Fallback to defaults if parsing fails
	if hookPlan == "" {
		hookPlan = "# AI generated HookPlan - parsing failed, please regenerate"
	}
	if ebpfSource == "" {
		ebpfSource = "// AI generated eBPF source - parsing failed, please regenerate"
	}
	if sigmaRules == "" {
		sigmaRules = "# AI generated Sigma rules - parsing failed, please regenerate"
	}
	if correlation == "" {
		correlation = "# AI generated Correlation - parsing failed, please regenerate"
	}
	return
}

func extractCodeBlock(response, lang, sectionHint string) string {
	lines := strings.Split(response, "\n")
	inSection := false
	var blockLines []string
	inBlock := false

	for _, line := range lines {
		lower := strings.ToLower(line)
		trimmed := strings.TrimSpace(line)

		// Match section headers: ## HookPlan, # eBPF Source Draft, etc.
		if strings.HasPrefix(trimmed, "#") && strings.Contains(lower, strings.ToLower(sectionHint)) {
			inSection = true
			continue
		}

		if inSection && strings.HasPrefix(trimmed, "```") {
			if inBlock {
				break
			}
			inBlock = true
			continue
		}

		if inSection && inBlock {
			blockLines = append(blockLines, line)
		}

		// If we hit another section header while looking, stop
		if inSection && !inBlock && strings.HasPrefix(trimmed, "## ") && !strings.Contains(lower, strings.ToLower(sectionHint)) {
			inSection = false
		}
	}

	if len(blockLines) > 0 {
		return strings.Join(blockLines, "\n")
	}
	return ""
}

// extractCodeBlocksByPosition extracts code blocks by language and position order
// Order: 1st yaml → HookPlan, 1st c → eBPF, 2nd yaml → Sigma, 3rd yaml → Correlation
func extractCodeBlocksByPosition(response string) (hookPlan, ebpfSource, sigmaRules, correlation string) {
	type codeBlock struct {
		lang string
		code string
	}
	var blocks []codeBlock

	lines := strings.Split(response, "\n")
	var currentLines []string
	inBlock := false
	currentLang := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inBlock {
				// End of block
				blocks = append(blocks, codeBlock{lang: currentLang, code: strings.Join(currentLines, "\n")})
				currentLines = nil
				inBlock = false
				currentLang = ""
			} else {
				// Start of block - extract language
				lang := strings.TrimPrefix(trimmed, "```")
				lang = strings.TrimSpace(lang)
				currentLang = strings.ToLower(lang)
				inBlock = true
			}
			continue
		}
		if inBlock {
			currentLines = append(currentLines, line)
		}
	}

	// Assign by position
	yamlIdx := 0
	for _, b := range blocks {
		if strings.Contains(b.lang, "yaml") || strings.Contains(b.lang, "yml") {
			switch yamlIdx {
			case 0:
				hookPlan = b.code
			case 1:
				sigmaRules = b.code
			case 2:
				correlation = b.code
			}
			yamlIdx++
		} else if strings.Contains(b.lang, "c") {
			ebpfSource = b.code
		}
	}
	return
}
```

- [ ] **Step 2: 验证编译通过**

Run: `cd /code/ai-benchmark/api-server && go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 3: Commit**

```bash
git add api-server/internal/api/handler/detection_package_handler.go
git commit -m "feat: 增强LLM响应解析器，支持位置fallback"
```

---

### Task 4: 增强前端AI生成错误反馈

**Files:**
- Modify: `frontend/src/views/detection/DetectionPackages/PackageEditor.vue:152-175`

- [ ] **Step 1: 更新confirmAIGenerate函数的错误处理**

替换 `PackageEditor.vue` 第152-175行：

```ts
async function confirmAIGenerate() {
  if (!aiForm.cve_id || !aiForm.vulnerability_description) {
    ElMessage.warning('请填写 CVE ID 和漏洞描述')
    return
  }
  generating.value = true
  try {
    const draft = await generateDraft(aiForm)
    if (draft) {
      form.package_id = draft.package_id
      form.target_version = draft.target_version
      form.title = draft.title
      form.description = draft.description || ''
      form.cve_ids = draft.cve_ids || []
      form.hook_plan_yaml = draft.hook_plan_yaml || ''
      form.ebpf_source = draft.ebpf_source || ''
      form.sigma_rules_yaml = draft.sigma_rules_yaml || ''
      form.correlation_yaml = draft.correlation_yaml || ''
      aiDialogVisible.value = false
    }
  } catch (e: any) {
    ElMessage.error(e?.message || 'AI 草稿生成失败，请检查LLM配置或稍后重试')
  } finally {
    generating.value = false
  }
}
```

注意：前端axios拦截器（`frontend/src/api/index.ts:29-55`）已经处理了HTTP 503错误状态码，会自动显示错误消息。此步骤增加catch块中的显式错误提示，确保双重保障。

- [ ] **Step 2: 验证前端编译通过**

Run: `cd /code/ai-benchmark/frontend && npm run build`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/detection/DetectionPackages/PackageEditor.vue
git commit -m "fix: 增强前端AI生成草稿错误反馈"
```

---

### Task 5: 更新设计文档

**Files:**
- Modify: `docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md`

- [ ] **Step 1: 更新AI生成草稿API文档**

在AI生成草稿API部分，更新以下内容：
- 超时配置为120秒
- LLM失败时返回503错误而非占位符
- 请求字段增加 `attack_prerequisites` 的说明
- 响应错误码增加503说明

- [ ] **Step 2: Commit**

```bash
git add docs/aegis_system_design_v5.8/api_grpc_design_v5.8.md
git commit -m "docs: 更新AI生成草稿API设计文档"
```

---

### Task 6: 编译启动服务并使用curl测试

**Files:**
- No new files

- [ ] **Step 1: 使用aegis-build-test技能编译api-server**

按照 `.agents/codex-skills/aegis-build-test/SKILL.md` 的流程编译api-server。

Run: `cd /code/ai-benchmark/api-server && make build`
Expected: 编译成功

- [ ] **Step 2: 重启api-server服务**

Run: `docker compose restart api-server`
Expected: 服务正常启动

- [ ] **Step 3: 获取认证token**

```bash
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('token',''))")
echo "Token: $TOKEN"
```

- [ ] **Step 4: 测试AI生成草稿接口（正常场景）**

```bash
curl -s -X POST http://localhost:8082/api/v1/detection/packages/ai-generate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "cve_id": "CVE-2026-31431",
    "vulnerability_description": "打开加密socket(AF_ALG, SOCK_SEQPACKET, 0)绑定到authencesn(hmac(sha256),cbc(aes))，设置密钥，接受操作socket u。全程零权限。一次写入一个4字节片段攻击者的shellcode被切分成以4字节为单位的小块。对每一块：u.sendmsg()发送构造好的AAD。AAD的[4..7]就是要写入的shellcode片段。os.splice()将目标文件的指定偏移内容导入管道，再导入AF_ALG socket。u.recv()触发解密。内核内部：authencesn将shellcode片段写入了su页缓存的预期位置，然后HMAC校验失败，返回-1。页缓存已经静默受损。",
    "attack_prerequisites": "需要本地普通用户权限，内核支持AF_ALG接口",
    "exploitation_chain": "1. 打开AF_ALG socket 2. 绑定authencesn算法 3. 设置密钥 4. sendmsg发送shellcode片段 5. splice导入目标文件内容 6. recv触发解密写入页缓存 7. execve执行被篡改的setuid二进制",
    "false_positive_constraints": "正常使用AF_ALG进行加密操作不应触发，需区分正常加密操作和利用行为"
  }' | python3 -m json.tool
```

Expected: 返回包含 `hook_plan_yaml`、`ebpf_source`、`sigma_rules_yaml`、`correlation_yaml` 的草稿数据，内容为LLM生成的真实代码而非占位符。

- [ ] **Step 5: 测试AI生成草稿接口（LLM未配置场景）**

如果LLM未配置，应返回503错误：

```bash
curl -s -X POST http://localhost:8082/api/v1/detection/packages/ai-generate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "cve_id": "CVE-2026-31431",
    "vulnerability_description": "test"
  }' | python3 -m json.tool
```

Expected: 如果LLM配置正确则返回草稿；如果LLM配置缺失则返回 `{"code": 503, "message": "AI生成服务未配置..."}`

- [ ] **Step 6: Commit测试结果记录**

无需commit，测试通过即可。
