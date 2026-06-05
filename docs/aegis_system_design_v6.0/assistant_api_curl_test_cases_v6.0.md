# Aegis V6.0 Assistant API curl 测试用例

**版本**: 6.0  
**日期**: 2026-06-04  
**状态**: 测试设计  
**范围**: V6.0 智能模式 Assistant HTTP API、SSE、工具审批模式、工具白名单、审批批准/拒绝、数据一致性

---

## 1. 测试目标

本文档给开发和测试人员提供一套可执行的 `curl + jq` 接口验收用例。每个用例必须同时校验：

1. HTTP 状态码是否符合预期。
2. JSON 返回结构是否符合 API 设计。
3. `code`、`data`、`status`、`total`、`items/tools` 等关键字段是否符合预期。
4. 写接口执行后，再通过查询接口确认数据已经落库且状态一致。
5. 审批模式切换后，智能体工具调用行为符合 `request_approval` / `whitelist` / `full_access` 规则。

测试接口范围：

| 模块 | 接口 |
|:---|:---|
| 会话 | `GET/POST /assistant/sessions`、`GET /assistant/sessions/:session_id` |
| 消息和运行 | `POST /assistant/sessions/:session_id/message`、`GET /assistant/sessions/:session_id/messages`、`POST /assistant/sessions/:session_id/cancel` |
| SSE | `GET /assistant/sessions/:session_id/stream` |
| 上下文 | `GET /assistant/sessions/:session_id/context-refs` |
| 工具调用 | `GET /assistant/sessions/:session_id/tool-calls` |
| 会话审批 | `GET /assistant/sessions/:session_id/approvals` |
| 工具配置 | `GET /assistant/tools` |
| 审批策略 | `GET/PUT /assistant/tool-approval-policy` |
| 白名单 | `PUT /assistant/tools/:tool_name/whitelist`、`POST /assistant/tools/whitelist/batch`、`POST /assistant/tools/whitelist/reset-defaults` |
| 审批详情 | `GET /assistant/approvals/:approval_id` |
| 审批动作 | `POST /assistant/approvals/:approval_id/approve`、`POST /assistant/approvals/:approval_id/reject` |
| 主机攻击研判 | `POST /assistant/investigations/host-attack`、`GET /assistant/investigations/:id`、`GET /assistant/investigations/:id/evidence` |
| 外接 MCP 数据源 | `GET/POST/PUT/DELETE /assistant/mcp-sources`、`POST /assistant/mcp-sources/:id/test`、`POST /assistant/mcp-sources/:id/sync-schema` |

---

## 2. 测试环境准备

### 2.1 依赖工具

测试机器需要安装：

```bash
curl --version
jq --version
```

### 2.2 环境变量

```bash
export BASE_URL="http://localhost:8082"
export API_PREFIX="$BASE_URL/api/v1"
export USERNAME="${AEGIS_USERNAME:-admin}"
export PASSWORD="${AEGIS_PASSWORD:?please export AEGIS_PASSWORD}"
```

说明：

- 不在文档中写死密码。
- `AEGIS_PASSWORD` 由测试执行人或 CI secret 注入。
- 如果环境使用 bootstrap 登录，先按现有认证流程完成密码初始化，再执行本文档用例。

### 2.3 获取 Token

```bash
TOKEN=$(
  curl -sS -X POST "$API_PREFIX/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" \
  | jq -r '.token'
)

test -n "$TOKEN" && test "$TOKEN" != "null"
```

预期：

- `TOKEN` 非空。
- `GET /auth/me` 能返回当前用户和角色。

```bash
curl -sS "$API_PREFIX/auth/me" \
  -H "Authorization: Bearer $TOKEN" \
| jq -e '.username == env.USERNAME and (.role | type == "string")'
```

### 2.4 通用 curl 断言函数

以下函数建议复制到 `tmp/assistant_api_test.sh` 后执行。开发人员也可以直接把每个用例拆到 CI。

```bash
set -euo pipefail

TMP_DIR="${TMPDIR:-/tmp}/aegis-assistant-api-test"
mkdir -p "$TMP_DIR"

http_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local out="$TMP_DIR/response.json"
  local code_file="$TMP_DIR/status.txt"

  if [ -n "$body" ]; then
    curl -sS -X "$method" "$API_PREFIX$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$body" \
      -o "$out" \
      -w "%{http_code}" > "$code_file"
  else
    curl -sS -X "$method" "$API_PREFIX$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -o "$out" \
      -w "%{http_code}" > "$code_file"
  fi

  export HTTP_STATUS
  HTTP_STATUS="$(cat "$code_file")"
  cat "$out"
}

assert_http() {
  local expected="$1"
  test "$HTTP_STATUS" = "$expected" || {
    echo "expected HTTP $expected, got $HTTP_STATUS"
    cat "$TMP_DIR/response.json"
    exit 1
  }
}

assert_json() {
  local expr="$1"
  jq -e "$expr" "$TMP_DIR/response.json" >/dev/null || {
    echo "jq assertion failed: $expr"
    cat "$TMP_DIR/response.json"
    exit 1
  }
}

assert_code0() {
  assert_json '.code == 0'
}
```

### 2.5 确定性测试数据要求

为了让 `curl` 用例可以稳定判断工具调用和审批结果，测试环境需要提供确定性数据和确定性智能体运行方式。

基础 fixture：

| 对象 | 建议 ID | 目的 |
|:---|:---|:---|
| Host | `host-curl-test-001` | 上下文引用、Host 查询 |
| Alert | `ALT-CURL-TEST-001` | 上下文引用、告警查询 |
| Vulnerability | `CVE-CURL-TEST-0001` | 主机攻击研判入口关联 |
| Baseline failed item | `baseline-curl-test-001` | 主机攻击研判弱配置关联 |
| DetectionPackage | `pkg-curl-test-001` | `Package.Sign` 审批拒绝测试 |
| DetectionPackage | `pkg-curl-test-approve-001` | `Package.Sign` 审批批准测试 |

确定性运行要求：

1. `ToolCatalog` 必须包含本文档引用的工具：`Host.List`、`Detection.Alert.List`、`Package.Sign`、`Package.Enable` 等。
2. `ToolCatalog` 必须包含 `Investigation.HostAttack.Analyze`、`Investigation.HostAttack.AnalyzeWithExternal`。
3. `ToolPolicyService.SyncCatalogTools(ctx)` 必须在 api-server 启动时完成。
4. CI 环境建议开启 `ASSISTANT_DETERMINISTIC_TEST=true`，让 agent-runtime 或测试 LLM 根据固定 prompt 返回固定工具调用。
5. 如果不启用确定性测试模式，`TC-MODE-*` 和 `TC-APPROVAL-*` 仍可作为人工验收脚本，但不能作为稳定 CI 阻塞项。
6. 对没有真实业务 fixture 的高风险工具，允许工具执行失败，但审批状态、工具调用状态和失败原因必须可查。

---

## 3. 基础连通性和认证

### TC-AUTH-001 健康检查

```bash
curl -sS "$BASE_URL/health" | jq -e '.status == "ok"'
```

预期：

- HTTP 200。
- `status=ok`。

### TC-AUTH-002 未带 Token 访问 Assistant 应返回 401

```bash
curl -sS -o "$TMP_DIR/no_auth.json" -w "%{http_code}" \
  "$API_PREFIX/assistant/sessions" > "$TMP_DIR/no_auth.status"

test "$(cat "$TMP_DIR/no_auth.status")" = "401"
jq -e '.message | type == "string"' "$TMP_DIR/no_auth.json"
```

预期：

- HTTP 401。
- 不返回会话数据。

---

## 4. 会话接口测试

### TC-SESSION-001 创建普通 Assistant 会话

```bash
CREATE_SESSION_BODY='{
  "title": "curl 测试智能模式会话",
  "task_type": "explanation",
  "initial_message": "总结当前系统态势",
  "context_refs": []
}'

http_json POST "/assistant/sessions" "$CREATE_SESSION_BODY" > "$TMP_DIR/create_session.out"
assert_http 200
assert_code0
assert_json '.data.session_id | test("^asst_")'
assert_json '.data.title == "curl 测试智能模式会话"'
assert_json '.data.task_type == "explanation"'
assert_json '.data.status == "active"'

export SESSION_ID
SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"
test -n "$SESSION_ID" && test "$SESSION_ID" != "null"
```

数据预期：

- 新增一条 `assistant_sessions`。
- `message_count >= 0`。
- 如果实现选择保存 `initial_message`，则后续 `GET /messages` 至少包含一条 user message。

### TC-SESSION-002 创建带上下文对象的会话

```bash
CREATE_CONTEXT_SESSION_BODY='{
  "title": "curl 测试带上下文会话",
  "task_type": "investigation",
  "initial_message": "分析这条告警",
  "context_refs": [
    { "object_type": "alert", "object_id": "ALT-CURL-TEST-001" },
    { "object_type": "host", "object_id": "host-curl-test-001" }
  ]
}'

http_json POST "/assistant/sessions" "$CREATE_CONTEXT_SESSION_BODY" > "$TMP_DIR/create_context_session.out"
assert_http 200
assert_code0
assert_json '.data.session_id | test("^asst_")'
assert_json '.data.task_type == "investigation"'

export CONTEXT_SESSION_ID
CONTEXT_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"
```

然后校验上下文是否可查：

```bash
http_json GET "/assistant/sessions/$CONTEXT_SESSION_ID/context-refs" > "$TMP_DIR/context_refs.out"
assert_http 200
assert_code0
assert_json '(.data.refs // .data.items // .data) | length >= 2'
assert_json '[(.data.refs // .data.items // .data)[] | .object_type] | index("alert") != null'
assert_json '[(.data.refs // .data.items // .data)[] | .object_type] | index("host") != null'
```

### TC-SESSION-003 查询会话列表

```bash
http_json GET "/assistant/sessions?page=1&page_size=20" > "$TMP_DIR/list_sessions.out"
assert_http 200
assert_code0
assert_json '.data.total >= 1'
assert_json '(.data.sessions // .data.items // .data.data) | type == "array"'
assert_json '[((.data.sessions // .data.items // .data.data)[] | .session_id)] | index(env.SESSION_ID) != null'
```

### TC-SESSION-004 查询会话详情

```bash
http_json GET "/assistant/sessions/$SESSION_ID" > "$TMP_DIR/get_session.out"
assert_http 200
assert_code0
assert_json '.data.session_id == env.SESSION_ID'
assert_json '.data.status == "active"'
assert_json '.data.title | type == "string"'
```

### TC-SESSION-005 查询不存在的会话

```bash
http_json GET "/assistant/sessions/asst_not_exists" > "$TMP_DIR/get_missing_session.out" || true
test "$HTTP_STATUS" = "404" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
else
  jq -e '.message | type == "string"' "$TMP_DIR/response.json"
fi
```

预期：

- 推荐实现为 HTTP 404。
- 如果沿用统一 `code` 错误结构，也必须 `code != 0`。

---

## 5. 工具配置和白名单接口测试

### TC-TOOLS-001 获取全部工具列表

```bash
http_json GET "/assistant/tools?page=1&page_size=100" > "$TMP_DIR/list_tools.out"
assert_http 200
assert_code0
assert_json '.data.mode | IN("request_approval", "whitelist", "full_access")'
assert_json '.data.total > 0'
assert_json '.data.tools | type == "array"'
assert_json '[.data.tools[].name] | index("Host.List") != null'
assert_json '[.data.tools[].name] | index("Package.Sign") != null'
assert_json '.data.tools[] | select(.name == "Package.Sign") | .risk_level == "critical"'
assert_json '.data.tools[] | select(.name == "Package.Sign") | .whitelisted == false'
```

数据预期：

- `ToolCatalog` 已同步到 `assistant_tool_policies`。
- `Host.List` 这类只读工具存在。
- `Package.Sign` 这类 critical 工具默认不在白名单。

### TC-TOOLS-002 按领域和风险过滤工具

```bash
http_json GET "/assistant/tools?domain=package&risk=critical&page=1&page_size=50" > "$TMP_DIR/filter_tools.out"
assert_http 200
assert_code0
assert_json '.data.tools | length >= 1'
assert_json 'all(.data.tools[]; .domain == "package" and .risk_level == "critical")'
```

### TC-TOOLS-003 搜索工具名称和详情

```bash
http_json GET "/assistant/tools?keyword=sign&page=1&page_size=20" > "$TMP_DIR/search_tools.out"
assert_http 200
assert_code0
assert_json '.data.tools | length >= 1'
assert_json 'any(.data.tools[]; .name == "Package.Sign")'
```

### TC-POLICY-001 获取当前审批策略

```bash
http_json GET "/assistant/tool-approval-policy" > "$TMP_DIR/get_policy.out"
assert_http 200
assert_code0
assert_json '.data.mode | IN("request_approval", "whitelist", "full_access")'
assert_json '.data.whitelist_version | type == "number" or .data.whitelist_version == null'
```

### TC-POLICY-002 切换到白名单模式

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"whitelist"}' > "$TMP_DIR/update_policy_whitelist.out"
assert_http 200
assert_code0
assert_json '.data.mode == "whitelist"'

http_json GET "/assistant/tool-approval-policy" > "$TMP_DIR/get_policy_after_whitelist.out"
assert_http 200
assert_code0
assert_json '.data.mode == "whitelist"'
```

数据预期：

- `system_configs.assistant.tool_approval_mode=whitelist`。
- 后续工具执行使用白名单策略。

### TC-POLICY-003 非法审批模式应失败

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"god_mode"}' > "$TMP_DIR/update_policy_invalid.out" || true
test "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
else
  jq -e '.message | type == "string" or .code != 0' "$TMP_DIR/response.json"
fi
```

预期：

- 不允许写入非法模式。
- 再查策略时仍为上一次合法值。

```bash
http_json GET "/assistant/tool-approval-policy" > "$TMP_DIR/get_policy_after_invalid.out"
assert_http 200
assert_code0
assert_json '.data.mode == "whitelist"'
```

### TC-WHITELIST-001 将低危只读工具加入白名单

```bash
http_json PUT "/assistant/tools/Host.List/whitelist" '{"whitelisted":true}' > "$TMP_DIR/whitelist_host_list.out"
assert_http 200
assert_code0
assert_json '.data.tool_name == "Host.List" or .data.name == "Host.List"'
assert_json '.data.whitelisted == true'

http_json GET "/assistant/tools?keyword=Host.List" > "$TMP_DIR/get_host_list_policy.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Host.List") | .whitelisted == true'
```

### TC-WHITELIST-002 critical 工具默认不应在白名单

```bash
http_json GET "/assistant/tools?keyword=Package.Sign" > "$TMP_DIR/get_package_sign_policy.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Package.Sign") | .risk_level == "critical"'
assert_json '.data.tools[] | select(.name == "Package.Sign") | .default_whitelisted == false'
```

### TC-WHITELIST-003 critical 工具可由管理员显式加入白名单

该用例验证配置能力，不代表推荐生产配置。

```bash
http_json PUT "/assistant/tools/Package.Sign/whitelist" '{"whitelisted":true}' > "$TMP_DIR/whitelist_package_sign.out"
assert_http 200
assert_code0
assert_json '.data.whitelisted == true'

http_json GET "/assistant/tools?keyword=Package.Sign" > "$TMP_DIR/get_package_sign_whitelisted.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Package.Sign") | .whitelisted == true'
```

恢复：

```bash
http_json PUT "/assistant/tools/Package.Sign/whitelist" '{"whitelisted":false}' > "$TMP_DIR/unwhitelist_package_sign.out"
assert_http 200
assert_code0
assert_json '.data.whitelisted == false'
```

### TC-WHITELIST-004 批量更新白名单

```bash
BATCH_WHITELIST_BODY='{
  "items": [
    { "tool_name": "Host.List", "whitelisted": true },
    { "tool_name": "Detection.Alert.List", "whitelisted": true },
    { "tool_name": "Package.Sign", "whitelisted": false }
  ]
}'

http_json POST "/assistant/tools/whitelist/batch" "$BATCH_WHITELIST_BODY" > "$TMP_DIR/batch_whitelist.out"
assert_http 200
assert_code0
assert_json '.data.updated_count >= 3 or (.data.items | length >= 3)'

http_json GET "/assistant/tools?page=1&page_size=200" > "$TMP_DIR/tools_after_batch.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Host.List") | .whitelisted == true'
assert_json '.data.tools[] | select(.name == "Detection.Alert.List") | .whitelisted == true'
assert_json '.data.tools[] | select(.name == "Package.Sign") | .whitelisted == false'
```

### TC-WHITELIST-005 恢复默认白名单

```bash
http_json POST "/assistant/tools/whitelist/reset-defaults" '{}' > "$TMP_DIR/reset_whitelist.out"
assert_http 200
assert_code0

http_json GET "/assistant/tools?page=1&page_size=200" > "$TMP_DIR/tools_after_reset.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Host.List") | .whitelisted == true'
assert_json '.data.tools[] | select(.name == "Package.Sign") | .whitelisted == false'
```

---

## 6. 消息、运行和 SSE 测试

### TC-MESSAGE-001 发送只读查询消息

先确保白名单模式和只读工具白名单：

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"whitelist"}' > "$TMP_DIR/set_whitelist_mode.out"
assert_http 200
assert_code0

http_json PUT "/assistant/tools/Host.List/whitelist" '{"whitelisted":true}' > "$TMP_DIR/set_host_list_whitelist.out"
assert_http 200
assert_code0
```

发送消息：

```bash
SEND_READONLY_BODY='{
  "content": "列出当前主机资产，返回数量和前 5 台主机",
  "context_refs": []
}'

http_json POST "/assistant/sessions/$SESSION_ID/message" "$SEND_READONLY_BODY" > "$TMP_DIR/send_readonly_message.out"
assert_http 200
assert_code0
assert_json '.data.message_id | test("^msg_")'
assert_json '.data.run_id | test("^run_")'
assert_json '.data.status | IN("running", "completed", "waiting_approval")'

export READONLY_RUN_ID
READONLY_RUN_ID="$(jq -r '.data.run_id' "$TMP_DIR/response.json")"
```

预期：

- 生成 `assistant_messages` user message。
- 生成 `assistant_runs`。
- 在白名单模式且 `Host.List` 白名单时，不应产生审批。

### TC-MESSAGE-002 查询消息历史

```bash
http_json GET "/assistant/sessions/$SESSION_ID/messages?page=1&page_size=50" > "$TMP_DIR/list_messages.out"
assert_http 200
assert_code0
assert_json '(.data.messages // .data.items // .data.data) | type == "array"'
assert_json 'any((.data.messages // .data.items // .data.data)[]; .message_id != null)'
```

数据预期：

- 至少包含刚刚发送的 user message。
- 如果 runtime 已完成，应该包含 assistant message。

### TC-MESSAGE-003 查询工具调用记录

```bash
http_json GET "/assistant/sessions/$SESSION_ID/tool-calls?page=1&page_size=50" > "$TMP_DIR/list_tool_calls.out"
assert_http 200
assert_code0
assert_json '(.data.tool_calls // .data.items // .data.data) | type == "array"'
```

如果环境启用了确定性测试 runtime，则继续断言：

```bash
if [ "${ASSISTANT_DETERMINISTIC_TEST:-false}" = "true" ]; then
  jq -e 'any((.data.tool_calls // .data.items // .data.data)[]; .tool_name == "Host.List" and (.status | IN("success", "completed")))' "$TMP_DIR/response.json"
fi
```

### TC-SSE-001 SSE 能返回事件流

```bash
timeout 15s curl -sS -N "$API_PREFIX/assistant/sessions/$SESSION_ID/stream" \
  -H "Authorization: Bearer $TOKEN" \
  > "$TMP_DIR/sse_events.txt" || true

test -s "$TMP_DIR/sse_events.txt"
grep -E "event:|data:" "$TMP_DIR/sse_events.txt" >/dev/null
```

如果已经发送过消息，预期至少出现以下事件之一：

```bash
grep -E "tools_selected|tool_call|tool_result|message_delta|done|approval_required|error" "$TMP_DIR/sse_events.txt" >/dev/null
```

### TC-RUN-001 取消当前 run

创建一个新会话并发送可能较长的任务：

```bash
http_json POST "/assistant/sessions" '{
  "title": "curl 测试取消 run",
  "task_type": "investigation",
  "initial_message": "持续分析最近 7 天告警并生成详细计划",
  "context_refs": []
}' > "$TMP_DIR/create_cancel_session.out"
assert_http 200
assert_code0

export CANCEL_SESSION_ID
CANCEL_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"

http_json POST "/assistant/sessions/$CANCEL_SESSION_ID/message" '{"content":"开始一个较长的分析任务"}' > "$TMP_DIR/send_cancel_message.out"
assert_http 200
assert_code0

http_json POST "/assistant/sessions/$CANCEL_SESSION_ID/cancel" '{}' > "$TMP_DIR/cancel_run.out"
assert_http 200
assert_code0
assert_json '.data.status | IN("cancelled", "canceled")'
```

再查会话：

```bash
http_json GET "/assistant/sessions/$CANCEL_SESSION_ID" > "$TMP_DIR/get_cancel_session.out"
assert_http 200
assert_code0
assert_json '.data.status | IN("active", "cancelled", "canceled")'
```

预期：

- 当前 run 停止。
- 后续不再产生新的工具调用。
- 已落库的消息和工具调用不丢失。

---

## 7. 三种审批模式行为测试

### TC-MODE-001 request_approval 模式下只读工具也要审批

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"request_approval"}' > "$TMP_DIR/set_request_approval.out"
assert_http 200
assert_code0
assert_json '.data.mode == "request_approval"'

http_json POST "/assistant/sessions" '{
  "title": "curl 请求批准模式",
  "task_type": "explanation",
  "initial_message": "列出当前主机资产",
  "context_refs": []
}' > "$TMP_DIR/create_request_mode_session.out"
assert_http 200
assert_code0

export REQUEST_MODE_SESSION_ID
REQUEST_MODE_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"

http_json POST "/assistant/sessions/$REQUEST_MODE_SESSION_ID/message" '{"content":"列出主机资产"}' > "$TMP_DIR/request_mode_message.out"
assert_http 200
assert_code0
assert_json '.data.status | IN("running", "waiting_approval")'
```

等待审批产生：

```bash
sleep 3
http_json GET "/assistant/sessions/$REQUEST_MODE_SESSION_ID/approvals?page=1&page_size=20" > "$TMP_DIR/request_mode_approvals.out"
assert_http 200
assert_code0
assert_json '(.data.approvals // .data.items // .data.data) | length >= 1'
assert_json 'any((.data.approvals // .data.items // .data.data)[]; .status == "pending")'
```

预期：

- 即使工具是 `Host.List` 或其他 readonly，也必须创建 pending 审批。
- run 状态应为 `waiting_approval` 或可从审批列表体现等待。

### TC-MODE-002 whitelist 模式下白名单工具免审批

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"whitelist"}' > "$TMP_DIR/set_whitelist_mode_again.out"
assert_http 200
assert_code0

http_json PUT "/assistant/tools/Host.List/whitelist" '{"whitelisted":true}' > "$TMP_DIR/ensure_host_whitelisted.out"
assert_http 200
assert_code0

http_json POST "/assistant/sessions" '{
  "title": "curl 白名单模式",
  "task_type": "explanation",
  "initial_message": "列出主机资产",
  "context_refs": []
}' > "$TMP_DIR/create_whitelist_mode_session.out"
assert_http 200
assert_code0

export WHITELIST_MODE_SESSION_ID
WHITELIST_MODE_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"

http_json POST "/assistant/sessions/$WHITELIST_MODE_SESSION_ID/message" '{"content":"列出主机资产"}' > "$TMP_DIR/whitelist_mode_message.out"
assert_http 200
assert_code0

sleep 3
http_json GET "/assistant/sessions/$WHITELIST_MODE_SESSION_ID/approvals?page=1&page_size=20" > "$TMP_DIR/whitelist_mode_approvals.out"
assert_http 200
assert_code0
assert_json '[(.data.approvals // .data.items // .data.data)[] | select(.tool_name == "Host.List" and .status == "pending")] | length == 0'
```

预期：

- `Host.List` 无 pending 审批。
- 工具调用记录中 `Host.List` 应为 `success/completed`。

### TC-MODE-003 whitelist 模式下非白名单 critical 工具需要审批

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"whitelist"}' > "$TMP_DIR/set_whitelist_mode_critical.out"
assert_http 200
assert_code0

http_json PUT "/assistant/tools/Package.Sign/whitelist" '{"whitelisted":false}' > "$TMP_DIR/ensure_sign_not_whitelisted.out"
assert_http 200
assert_code0

http_json POST "/assistant/sessions" '{
  "title": "curl critical 审批",
  "task_type": "operations",
  "initial_message": "签名测试检测包",
  "context_refs": [
    { "object_type": "detection_package", "object_id": "pkg-curl-test-001" }
  ]
}' > "$TMP_DIR/create_critical_session.out"
assert_http 200
assert_code0

export CRITICAL_SESSION_ID
CRITICAL_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"

http_json POST "/assistant/sessions/$CRITICAL_SESSION_ID/message" '{"content":"签名这个检测包"}' > "$TMP_DIR/critical_message.out"
assert_http 200
assert_code0

sleep 3
http_json GET "/assistant/sessions/$CRITICAL_SESSION_ID/approvals?page=1&page_size=20" > "$TMP_DIR/critical_approvals.out"
assert_http 200
assert_code0
assert_json 'any((.data.approvals // .data.items // .data.data)[]; .tool_name == "Package.Sign" and .risk_level == "critical" and .status == "pending")'
```

预期：

- 不直接执行 `Package.Sign`。
- 审批卡片必须包含影响摘要、参数预览、回滚提示或风险说明。

```bash
assert_json '(.data.approvals // .data.items // .data.data)[] | select(.tool_name == "Package.Sign") | (.impact_summary // .summary // .draft.impact_summary) | type == "string"'
```

### TC-MODE-004 full_access 模式下跳过工具审批但保留审计

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"full_access"}' > "$TMP_DIR/set_full_access.out"
assert_http 200
assert_code0
assert_json '.data.mode == "full_access"'

http_json POST "/assistant/sessions" '{
  "title": "curl 完全权限模式",
  "task_type": "explanation",
  "initial_message": "列出主机资产",
  "context_refs": []
}' > "$TMP_DIR/create_full_access_session.out"
assert_http 200
assert_code0

export FULL_ACCESS_SESSION_ID
FULL_ACCESS_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"

http_json POST "/assistant/sessions/$FULL_ACCESS_SESSION_ID/message" '{"content":"列出主机资产"}' > "$TMP_DIR/full_access_message.out"
assert_http 200
assert_code0

sleep 3
http_json GET "/assistant/sessions/$FULL_ACCESS_SESSION_ID/approvals?page=1&page_size=20" > "$TMP_DIR/full_access_approvals.out"
assert_http 200
assert_code0
assert_json '[(.data.approvals // .data.items // .data.data)[] | select(.status == "pending")] | length == 0'

http_json GET "/assistant/sessions/$FULL_ACCESS_SESSION_ID/tool-calls?page=1&page_size=50" > "$TMP_DIR/full_access_tool_calls.out"
assert_http 200
assert_code0
assert_json '(.data.tool_calls // .data.items // .data.data) | type == "array"'
```

预期：

- 不产生 pending 审批。
- 仍然产生 `assistant_tool_calls` 记录。
- 仍然不能执行未注册、禁用或未被本轮选中的工具。

恢复默认：

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"whitelist"}' > "$TMP_DIR/restore_whitelist.out"
assert_http 200
assert_code0
```

---

## 8. 审批接口测试

### TC-APPROVAL-001 获取审批详情

先复用 `TC-MODE-003` 生成的 pending 审批：

```bash
http_json GET "/assistant/sessions/$CRITICAL_SESSION_ID/approvals?page=1&page_size=20" > "$TMP_DIR/find_pending_approval.out"
assert_http 200
assert_code0

export APPROVAL_ID
APPROVAL_ID="$(
  jq -r '(.data.approvals // .data.items // .data.data)[] | select(.status == "pending") | .approval_id' "$TMP_DIR/response.json" | head -n 1
)"
test -n "$APPROVAL_ID" && test "$APPROVAL_ID" != "null"

http_json GET "/assistant/approvals/$APPROVAL_ID" > "$TMP_DIR/get_approval.out"
assert_http 200
assert_code0
assert_json '.data.approval_id == env.APPROVAL_ID'
assert_json '.data.status == "pending"'
assert_json '.data.tool_name | type == "string"'
```

### TC-APPROVAL-002 拒绝审批

```bash
http_json POST "/assistant/approvals/$APPROVAL_ID/reject" '{"comment":"curl 测试拒绝，不执行高危动作"}' > "$TMP_DIR/reject_approval.out"
assert_http 200
assert_code0
assert_json '.data.approval_id == env.APPROVAL_ID'
assert_json '.data.status == "rejected"'

http_json GET "/assistant/approvals/$APPROVAL_ID" > "$TMP_DIR/get_rejected_approval.out"
assert_http 200
assert_code0
assert_json '.data.status == "rejected"'
```

数据预期：

- 原工具不执行。
- `assistant_tool_calls.status` 应变为 `rejected` 或等效失败状态。
- run 不应永久停留在 `waiting_approval`。

### TC-APPROVAL-003 批准审批并执行

重新创建一个 pending 审批：

```bash
http_json POST "/assistant/sessions" '{
  "title": "curl 批准审批",
  "task_type": "operations",
  "initial_message": "签名测试检测包",
  "context_refs": [
    { "object_type": "detection_package", "object_id": "pkg-curl-test-approve-001" }
  ]
}' > "$TMP_DIR/create_approve_session.out"
assert_http 200
assert_code0

export APPROVE_SESSION_ID
APPROVE_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"

http_json POST "/assistant/sessions/$APPROVE_SESSION_ID/message" '{"content":"签名这个检测包"}' > "$TMP_DIR/approve_session_message.out"
assert_http 200
assert_code0

sleep 3
http_json GET "/assistant/sessions/$APPROVE_SESSION_ID/approvals?page=1&page_size=20" > "$TMP_DIR/approve_session_approvals.out"
assert_http 200
assert_code0

export APPROVE_ID
APPROVE_ID="$(
  jq -r '(.data.approvals // .data.items // .data.data)[] | select(.status == "pending") | .approval_id' "$TMP_DIR/response.json" | head -n 1
)"
test -n "$APPROVE_ID" && test "$APPROVE_ID" != "null"
```

批准：

```bash
http_json POST "/assistant/approvals/$APPROVE_ID/approve" '{"comment":"curl 测试批准"}' > "$TMP_DIR/approve_approval.out"
assert_http 200
assert_code0
assert_json '.data.approval_id == env.APPROVE_ID'
assert_json '.data.status | IN("approved", "executed")'
assert_json '.data.execution_result.success | type == "boolean"'
```

执行结果校验：

```bash
http_json GET "/assistant/approvals/$APPROVE_ID" > "$TMP_DIR/get_approved_approval.out"
assert_http 200
assert_code0
assert_json '.data.status | IN("approved", "executed", "failed")'
assert_json '.data.tool_name | type == "string"'

http_json GET "/assistant/sessions/$APPROVE_SESSION_ID/tool-calls?page=1&page_size=50" > "$TMP_DIR/approved_tool_calls.out"
assert_http 200
assert_code0
assert_json 'any((.data.tool_calls // .data.items // .data.data)[]; .approval_id == env.APPROVE_ID or .tool_name == "Package.Sign")'
```

说明：

- 如果测试环境没有真实 `pkg-curl-test-approve-001`，工具执行可以失败，但审批状态必须从 `pending` 离开，且工具调用记录必须有失败原因。
- 如果测试环境有完整 fixture，预期 `status=executed` 且 `execution_result.success=true`。

### TC-APPROVAL-004 已拒绝或已执行审批不可重复操作

```bash
http_json POST "/assistant/approvals/$APPROVAL_ID/reject" '{"comment":"重复拒绝"}' > "$TMP_DIR/reject_again.out" || true
test "$HTTP_STATUS" = "409" -o "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0 or .data.status == "rejected"' "$TMP_DIR/response.json"
fi

http_json POST "/assistant/approvals/$APPROVE_ID/approve" '{"comment":"重复批准"}' > "$TMP_DIR/approve_again.out" || true
test "$HTTP_STATUS" = "409" -o "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0 or (.data.status | IN("executed", "failed"))' "$TMP_DIR/response.json"
fi
```

预期：

- 审批执行必须幂等。
- 不允许重复执行同一个工具调用。

---

## 9. 数据一致性测试

### TC-DATA-001 会话计数字段和列表一致

```bash
http_json GET "/assistant/sessions/$SESSION_ID" > "$TMP_DIR/session_count_detail.out"
assert_http 200
assert_code0

export SESSION_MESSAGE_COUNT
SESSION_MESSAGE_COUNT="$(jq -r '.data.message_count // 0' "$TMP_DIR/response.json")"
export SESSION_TOOL_CALL_COUNT
SESSION_TOOL_CALL_COUNT="$(jq -r '.data.tool_call_count // 0' "$TMP_DIR/response.json")"
export SESSION_APPROVAL_COUNT
SESSION_APPROVAL_COUNT="$(jq -r '.data.approval_count // 0' "$TMP_DIR/response.json")"

http_json GET "/assistant/sessions/$SESSION_ID/messages?page=1&page_size=100" > "$TMP_DIR/session_messages_for_count.out"
assert_http 200
assert_code0
jq -e --argjson expected "$SESSION_MESSAGE_COUNT" '
  ((.data.messages // .data.items // .data.data) | length) <= 100
  and (.data.total // expected) >= expected
' "$TMP_DIR/response.json"

http_json GET "/assistant/sessions/$SESSION_ID/tool-calls?page=1&page_size=100" > "$TMP_DIR/session_tool_calls_for_count.out"
assert_http 200
assert_code0
jq -e --argjson expected "$SESSION_TOOL_CALL_COUNT" '
  (.data.total // expected) >= expected
' "$TMP_DIR/response.json"

http_json GET "/assistant/sessions/$SESSION_ID/approvals?page=1&page_size=100" > "$TMP_DIR/session_approvals_for_count.out"
assert_http 200
assert_code0
jq -e --argjson expected "$SESSION_APPROVAL_COUNT" '
  (.data.total // expected) >= expected
' "$TMP_DIR/response.json"
```

预期：

- 详情计数字段不能大于对应列表总数。
- 分页返回的 `total` 与详情计数字段保持一致或更大。

### TC-DATA-002 工具策略修改后工具列表立即反映

```bash
http_json PUT "/assistant/tools/Host.List/whitelist" '{"whitelisted":false}' > "$TMP_DIR/host_list_off.out"
assert_http 200
assert_code0

http_json GET "/assistant/tools?keyword=Host.List" > "$TMP_DIR/host_list_after_off.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Host.List") | .whitelisted == false'

http_json PUT "/assistant/tools/Host.List/whitelist" '{"whitelisted":true}' > "$TMP_DIR/host_list_on.out"
assert_http 200
assert_code0

http_json GET "/assistant/tools?keyword=Host.List" > "$TMP_DIR/host_list_after_on.out"
assert_http 200
assert_code0
assert_json '.data.tools[] | select(.name == "Host.List") | .whitelisted == true'
```

预期：

- `UpdateWhitelist` 后无需重启 api-server。
- 配置页重新查询立即看到最新值。

### TC-DATA-003 审批批准后工具结果回填到审批和工具调用

该用例依赖 `TC-APPROVAL-003`。

```bash
http_json GET "/assistant/approvals/$APPROVE_ID" > "$TMP_DIR/approval_result_consistency.out"
assert_http 200
assert_code0

export APPROVAL_TOOL_CALL_ID
APPROVAL_TOOL_CALL_ID="$(jq -r '.data.tool_call_id // empty' "$TMP_DIR/response.json")"

http_json GET "/assistant/sessions/$APPROVE_SESSION_ID/tool-calls?page=1&page_size=100" > "$TMP_DIR/tool_result_consistency.out"
assert_http 200
assert_code0

if [ -n "$APPROVAL_TOOL_CALL_ID" ]; then
  jq -e --arg call_id "$APPROVAL_TOOL_CALL_ID" '
    any((.data.tool_calls // .data.items // .data.data)[]; .call_id == $call_id and (.status | IN("success", "completed", "failed")))
  ' "$TMP_DIR/response.json"
else
  jq -e 'any((.data.tool_calls // .data.items // .data.data)[]; .tool_name == "Package.Sign")' "$TMP_DIR/response.json"
fi
```

预期：

- 审批和工具调用能互相关联。
- 工具结果或失败原因可追溯。

---

## 10. 异常和边界测试

### TC-NEG-001 创建会话缺少 title

```bash
http_json POST "/assistant/sessions" '{"task_type":"explanation"}' > "$TMP_DIR/create_session_missing_title.out" || true
test "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
else
  jq -e '.message | type == "string" or .code != 0' "$TMP_DIR/response.json"
fi
```

### TC-NEG-002 发送空消息

```bash
http_json POST "/assistant/sessions/$SESSION_ID/message" '{"content":""}' > "$TMP_DIR/send_empty_message.out" || true
test "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
fi
```

### TC-NEG-003 更新不存在工具的白名单

```bash
http_json PUT "/assistant/tools/Not.Exists/whitelist" '{"whitelisted":true}' > "$TMP_DIR/update_missing_tool.out" || true
test "$HTTP_STATUS" = "404" -o "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
fi
```

### TC-NEG-004 批量白名单包含非法项

```bash
http_json POST "/assistant/tools/whitelist/batch" '{
  "items": [
    { "tool_name": "Host.List", "whitelisted": true },
    { "tool_name": "", "whitelisted": true }
  ]
}' > "$TMP_DIR/batch_invalid_tool.out" || true
test "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
fi
```

预期：

- 推荐整个批次失败，避免部分成功导致配置不可预期。
- 如果实现支持部分成功，必须返回 `failed_items` 并明确哪些项失败。

### TC-NEG-005 获取不存在审批

```bash
http_json GET "/assistant/approvals/appr_not_exists" > "$TMP_DIR/get_missing_approval.out" || true
test "$HTTP_STATUS" = "404" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0' "$TMP_DIR/response.json"
fi
```

---

## 11. 主机攻击研判测试

### TC-HAI-001 创建主机攻击研判会话

```bash
http_json POST "/assistant/sessions" '{
  "title": "curl 主机攻击研判",
  "task_type": "host_attack_investigation",
  "initial_message": "分析 host-curl-test-001 是否被攻击，入口是什么",
  "context_refs": [
    { "object_type": "host", "object_id": "host-curl-test-001" },
    { "object_type": "alert", "object_id": "ALT-CURL-TEST-001" }
  ]
}' > "$TMP_DIR/create_hai_session.out"
assert_http 200
assert_code0
assert_json '.data.task_type == "host_attack_investigation"'

export HAI_SESSION_ID
HAI_SESSION_ID="$(jq -r '.data.session_id' "$TMP_DIR/response.json")"
```

预期：

- 会话创建成功。
- 上下文包含 host 和 alert。

### TC-HAI-002 显式创建主机攻击研判

```bash
CREATE_HAI_BODY='{
  "session_id": "'"$HAI_SESSION_ID"'",
  "host_id": "host-curl-test-001",
  "alert_ids": ["ALT-CURL-TEST-001"],
  "time_range": {
    "from": "2026-06-04T00:00:00+08:00",
    "to": "2026-06-05T00:00:00+08:00"
  },
  "include_agent_live": true,
  "include_external_mcp": false,
  "mcp_source_ids": [],
  "max_evidence_items": 200
}'

http_json POST "/assistant/investigations/host-attack" "$CREATE_HAI_BODY" > "$TMP_DIR/create_hai.out"
assert_http 200
assert_code0
assert_json '.data.investigation_id | test("^inv_")'
assert_json '.data.compromise_assessment.verdict | IN("confirmed_compromised", "suspicious", "likely_benign", "insufficient_evidence")'
assert_json '.data.compromise_assessment.score >= 0 and .data.compromise_assessment.score <= 100'
assert_json '.data.compromise_assessment.confidence >= 0 and .data.compromise_assessment.confidence <= 1'
assert_json '.data.entry_point_candidates | type == "array"'
assert_json '.data.attack_timeline.events | type == "array"'
assert_json '.data.attack_path.nodes | type == "array"'
assert_json '.data.attack_path.edges | type == "array"'
assert_json '.data.evidence_matrix.items | type == "array"'

export HAI_INVESTIGATION_ID
HAI_INVESTIGATION_ID="$(jq -r '.data.investigation_id' "$TMP_DIR/response.json")"
```

证据一致性断言：

```bash
jq -e '
  if (.data.evidence_matrix.items | length) == 0
  then .data.compromise_assessment.verdict == "insufficient_evidence"
  else true
  end
' "$TMP_DIR/response.json"
```

预期：

- 不允许在无证据时输出确认性失陷。
- `score`、`confidence` 必须是可比较数值。

### TC-HAI-003 查询研判报告

```bash
http_json GET "/assistant/investigations/$HAI_INVESTIGATION_ID" > "$TMP_DIR/get_hai.out"
assert_http 200
assert_code0
assert_json '.data.investigation_id == env.HAI_INVESTIGATION_ID'
assert_json '.data.compromise_assessment.verdict | IN("confirmed_compromised", "suspicious", "likely_benign", "insufficient_evidence")'
assert_json '.data.report_markdown | type == "string"'
```

预期：

- 返回结构和创建接口一致。
- `report_markdown` 可以为空字符串，但字段必须存在。

### TC-HAI-004 查询研判证据

```bash
http_json GET "/assistant/investigations/$HAI_INVESTIGATION_ID/evidence?page=1&page_size=50" > "$TMP_DIR/list_hai_evidence.out"
assert_http 200
assert_code0
assert_json '(.data.items // .data.data // []) | type == "array"'
assert_json '(.data.total // 0) >= 0'
```

如果存在证据，继续断言：

```bash
jq -e '
  ((.data.items // .data.data // []) | length) == 0 or
  all((.data.items // .data.data // [])[]; (.evidence_id | type == "string") and (.source_type | type == "string") and (.summary | type == "string"))
' "$TMP_DIR/response.json"
```

### TC-HAI-005 对话触发研判工具选择

```bash
http_json POST "/assistant/sessions/$HAI_SESSION_ID/message" '{"content":"请判断这台主机是否被攻击，入口是什么，攻击是怎么进行的"}' > "$TMP_DIR/send_hai_message.out"
assert_http 200
assert_code0
assert_json '.data.run_id | test("^run_")'

sleep 3
http_json GET "/assistant/sessions/$HAI_SESSION_ID/tool-calls?page=1&page_size=100" > "$TMP_DIR/hai_tool_calls.out"
assert_http 200
assert_code0
```

确定性测试模式下继续断言：

```bash
if [ "${ASSISTANT_DETERMINISTIC_TEST:-false}" = "true" ]; then
  jq -e 'any((.data.tool_calls // .data.items // .data.data)[]; .tool_name == "Investigation.HostAttack.Analyze")' "$TMP_DIR/response.json"
fi
```

### TC-HAI-006 外部 MCP 研判失败不应编造证据

该用例依赖 TC-MCP-001 创建的数据源，也允许在无真实 MCP 服务时验证失败路径。

```bash
if [ -n "${MCP_SOURCE_ID:-}" ]; then
  CREATE_HAI_EXTERNAL_BODY='{
    "session_id": "'"$HAI_SESSION_ID"'",
    "host_id": "host-curl-test-001",
    "alert_ids": ["ALT-CURL-TEST-001"],
    "include_agent_live": false,
    "include_external_mcp": true,
    "mcp_source_ids": ["'"$MCP_SOURCE_ID"'"],
    "max_evidence_items": 200
  }'

  http_json POST "/assistant/investigations/host-attack" "$CREATE_HAI_EXTERNAL_BODY" > "$TMP_DIR/create_hai_external.out" || true
  test "$HTTP_STATUS" = "200" -o "$HTTP_STATUS" = "202" -o "$HTTP_STATUS" = "400" -o "$HTTP_STATUS" = "403"
  if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "202" ]; then
    jq -e '.data.missing_evidence | type == "array"' "$TMP_DIR/response.json"
    jq -e '(.data.source_coverage // {}) | type == "object"' "$TMP_DIR/response.json"
  else
    jq -e '.message | type == "string" or .code != 0' "$TMP_DIR/response.json"
  fi
fi
```

预期：

- 外部 MCP 不可用时，返回错误或 `missing_evidence`。
- 最终报告不能伪造外部 SIEM/CMDB/EDR 证据。

---

## 12. 外接 MCP 数据源配置测试

### TC-MCP-001 创建外接 MCP 数据源

```bash
CREATE_MCP_SOURCE_BODY='{
  "name": "curl-test-siem",
  "source_type": "siem",
  "transport": "streamable_http",
  "endpoint_url": "https://mcp.example.test/sse",
  "auth_type": "bearer",
  "credential": {
    "token": "curl-test-token"
  },
  "description": "curl 测试 MCP 数据源",
  "query_limits": {
    "max_rows": 50,
    "timeout_seconds": 10,
    "max_context_chars": 8000
  }
}'

http_json POST "/assistant/mcp-sources" "$CREATE_MCP_SOURCE_BODY" > "$TMP_DIR/create_mcp_source.out"
assert_http 200
assert_code0
assert_json '.data.source_id | test("^mcp_")'
assert_json '.data.name == "curl-test-siem"'
assert_json '.data.credential_configured == true'
assert_json '(.data | tostring) | contains("curl-test-token") | not'

export MCP_SOURCE_ID
MCP_SOURCE_ID="$(jq -r '.data.source_id' "$TMP_DIR/response.json")"
```

预期：

- 返回中不能包含明文 token。
- `external_mcp_sources` 新增一条记录。
- 凭据只通过 `credential_ref` 指向加密存储。

### TC-MCP-002 查询 MCP 数据源列表

```bash
http_json GET "/assistant/mcp-sources?page=1&page_size=20" > "$TMP_DIR/list_mcp_sources.out"
assert_http 200
assert_code0
assert_json '.data.total >= 1'
assert_json 'any((.data.items // .data.sources // .data.data)[]; .source_id == env.MCP_SOURCE_ID)'
assert_json '[(.data.items // .data.sources // .data.data)[] | tostring] | join(" ") | contains("curl-test-token") | not'
```

### TC-MCP-003 测试 MCP 连接

```bash
http_json POST "/assistant/mcp-sources/$MCP_SOURCE_ID/test" '{}' > "$TMP_DIR/test_mcp_source.out" || true
test "$HTTP_STATUS" = "200" -o "$HTTP_STATUS" = "502" -o "$HTTP_STATUS" = "504"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code == 0 and (.data.success | type == "boolean")' "$TMP_DIR/response.json"
else
  jq -e '.message | type == "string" or .code != 0' "$TMP_DIR/response.json"
fi
```

说明：

- CI 如果没有真实 MCP mock server，允许连接失败。
- 即使失败，也必须返回明确错误，不允许 500。

### TC-MCP-004 同步 schema

```bash
http_json POST "/assistant/mcp-sources/$MCP_SOURCE_ID/sync-schema" '{}' > "$TMP_DIR/sync_mcp_schema.out" || true
test "$HTTP_STATUS" = "200" -o "$HTTP_STATUS" = "502" -o "$HTTP_STATUS" = "504"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code == 0 and (.data.source_id == env.MCP_SOURCE_ID)' "$TMP_DIR/response.json"
fi
```

### TC-MCP-005 查询 MCP 调用日志

```bash
http_json GET "/assistant/mcp-sources/$MCP_SOURCE_ID/query-logs?page=1&page_size=20" > "$TMP_DIR/list_mcp_query_logs.out"
assert_http 200
assert_code0
assert_json '(.data.items // .data.logs // .data.data) | type == "array"'
```

### TC-MCP-006 删除 MCP 数据源

```bash
http_json DELETE "/assistant/mcp-sources/$MCP_SOURCE_ID" > "$TMP_DIR/delete_mcp_source.out"
assert_http 200
assert_code0

http_json GET "/assistant/mcp-sources/$MCP_SOURCE_ID" > "$TMP_DIR/get_deleted_mcp_source.out" || true
test "$HTTP_STATUS" = "404" -o "$HTTP_STATUS" = "200"
if [ "$HTTP_STATUS" = "200" ]; then
  jq -e '.code != 0 or .data.enabled == false' "$TMP_DIR/response.json"
fi
```

---

## 13. 建议 CI 执行顺序

```text
1. TC-AUTH-001 ~ TC-AUTH-002
2. TC-SESSION-001 ~ TC-SESSION-005
3. TC-TOOLS-001 ~ TC-TOOLS-003
4. TC-POLICY-001 ~ TC-POLICY-003
5. TC-WHITELIST-001 ~ TC-WHITELIST-005
6. TC-MESSAGE-001 ~ TC-SSE-001
7. TC-MODE-001 ~ TC-MODE-004
8. TC-APPROVAL-001 ~ TC-APPROVAL-004
9. TC-DATA-001 ~ TC-DATA-003
10. TC-NEG-001 ~ TC-NEG-005
11. TC-HAI-001 ~ TC-HAI-006
12. TC-MCP-001 ~ TC-MCP-006
13. 恢复默认配置: mode=whitelist, reset-defaults
```

收尾命令：

```bash
http_json PUT "/assistant/tool-approval-policy" '{"mode":"whitelist"}' > "$TMP_DIR/final_restore_policy.out"
assert_http 200
assert_code0

http_json POST "/assistant/tools/whitelist/reset-defaults" '{}' > "$TMP_DIR/final_restore_whitelist.out"
assert_http 200
assert_code0
```

---

## 14. 开发完成判定标准

开发人员完成 V6.0 Assistant API 后，必须满足：

1. 本文档所有正向用例通过。
2. 所有负向用例返回明确错误，不出现 500。
3. 审批模式切换即时生效，不需要重启。
4. 白名单配置在工具列表和实际工具执行中一致生效。
5. `request_approval` 模式下所有工具都创建审批。
6. `whitelist` 模式下白名单工具免审批，白名单外工具审批。
7. `full_access` 模式下跳过工具审批，但保留工具调用和业务审计。
8. 审批批准/拒绝后 run 不会永久卡住。
9. 工具执行成功或失败都能在 `tool-calls` 中查到结果。
10. 所有接口均可通过 `curl` 和 `jq` 在无前端环境下完成验收。
11. 主机攻击研判返回 verdict、score、entry candidates、timeline、attack path、evidence matrix。
12. 主机攻击研判无证据时必须返回 `insufficient_evidence`，不能确认失陷。
13. 外接 MCP 数据源接口返回不包含明文凭据。
14. 外接 MCP 连接失败时返回明确错误，不出现 500。
