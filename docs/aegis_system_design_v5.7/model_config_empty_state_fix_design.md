# V5.7 模型配置空状态修复设计

**版本**: 5.7
**状态**: 设计中
**范围**: 模型配置接口空状态行为、后端 handler/repository 测试

---

## 1. 问题说明

### 1.1 现象

模型配置页首次进入时，前端会分别请求：

- `GET /config/llm`
- `GET /config/image-model`

当数据库中不存在 active 配置时，接口当前返回：

```json
{
  "code": 500,
  "message": "failed to get config"
}
```

前端 Axios 拦截器会将 `code != 0` 识别为业务错误并弹出错误提示，导致用户在首次配置前就看到失败 toast。

### 1.2 用户体验问题

从设计师视角看，"未配置"是模型配置页的正常初始状态，不应被表达成错误状态。首次进入页面时，用户期望看到一个可编辑的空表单，并直接填写、保存配置。

当前行为的问题是：

- 空库初始化场景被误判为系统异常。
- 页面在用户尚未操作前弹错，破坏首次配置引导。
- 前端需要额外区分 500 中的"未配置"语义，增加不必要的 UI 分支。

---

## 2. 目标行为

### 2.1 未配置时返回默认占位配置

当 `llm` 或 `image-model` 没有 active 配置时，接口仍应返回 `code=0`，并给出默认占位配置。

响应语义：

- `code`: `0`
- `api_key_masked`: 空字符串
- `is_active`: `false`
- 其他字段返回页面可直接渲染和编辑的默认值

示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "provider": "",
    "model": "",
    "base_url": "",
    "api_key_masked": "",
    "is_active": false
  }
}
```

字段名以现有接口 DTO 为准；设计要求是保留既有响应结构，只改变"无 active 配置"时的业务语义。

### 2.2 页面可直接编辑保存

前端拿到默认占位配置后，应能按普通表单状态渲染：

- 不弹错误 toast。
- API Key 输入框为空。
- 页面按钮、表单校验、保存流程与已有配置场景一致。
- 用户保存后，后端创建或更新真实配置，并将其标记为 active。

---

## 3. 范围边界

### 3.1 本次范围

本次修复聚焦后端接口行为和测试：

- `GET /config/llm` 在无 active 配置时返回 `code=0` 的默认占位配置。
- `GET /config/image-model` 在无 active 配置时返回 `code=0` 的默认占位配置。
- handler 层覆盖空状态响应测试。
- repository/service 层覆盖"未找到 active 配置"与真实数据库错误的区分测试。
- 保存配置后，再次 GET 应返回真实配置与 masked API key。

### 3.2 不在本次范围

- 不引入数据库默认 API key。
- 不在迁移脚本中插入带密钥的默认模型配置。
- 不要求前端绕过拦截器或特殊处理 `failed to get config`。
- 不改变已有保存接口的鉴权、加密、脱敏策略。
- 不改变已有配置表结构，除非后端实现发现当前结构无法表达 active 配置。

---

## 4. 接口行为设计

### 4.1 GET /config/llm

| 场景 | 期望响应 | 说明 |
| --- | --- | --- |
| 存在 active LLM 配置 | `code=0`，返回真实配置 | API key 只返回 masked 值 |
| 不存在 active LLM 配置 | `code=0`，返回默认占位配置 | `api_key_masked=""`，`is_active=false` |
| 数据库查询异常 | `code=500` 或现有错误码 | 仅真实系统错误才弹错 |

### 4.2 GET /config/image-model

| 场景 | 期望响应 | 说明 |
| --- | --- | --- |
| 存在 active image model 配置 | `code=0`，返回真实配置 | API key 只返回 masked 值 |
| 不存在 active image model 配置 | `code=0`，返回默认占位配置 | `api_key_masked=""`，`is_active=false` |
| 数据库查询异常 | `code=500` 或现有错误码 | 仅真实系统错误才弹错 |

### 4.3 错误语义

repository/service 应区分两类情况：

- `record not found`: 正常空状态，转换为默认占位配置。
- 其他数据库错误: 系统异常，按现有错误响应返回。

这能避免把初始化空库误报为失败，同时保留真实错误的可观测性。

---

## 5. 默认占位配置要求

默认占位配置只用于接口响应，不应写入数据库。

最低字段要求：

| 字段 | 值 | 说明 |
| --- | --- | --- |
| `api_key_masked` | `""` | 未配置时不展示任何 masked 内容 |
| `is_active` | `false` | 明确表达当前不是已启用配置 |
| 文本模型 `provider` / `base_url` / `model_name` | `deepseek` / `https://api.deepseek.com/v1` / `deepseek-chat` | 对齐当前文本模型表单默认值 |
| 图片模型 `provider` / `base_url` / `model_name` | `zhipu` / `https://open.bigmodel.cn/api/paas/v4` / `cogview-3-flash` | 对齐当前图片模型表单默认值 |

如果现有 DTO 包含温度、超时、最大 token 等参数，可沿用页面当前默认值或后端既有默认值；但不得生成默认密钥。

---

## 6. 测试设计

### 6.1 Handler 测试

- 空库请求 `GET /config/llm`，断言 HTTP 成功、业务 `code=0`。
- 空库请求 `GET /config/image-model`，断言 HTTP 成功、业务 `code=0`。
- 响应体中 `api_key_masked=""`。
- 响应体中 `is_active=false`。

### 6.2 Repository/Service 测试

- 无 active 配置时，不向上返回可触发 `failed to get config` 的业务错误。
- 无 active 配置时，构造默认占位 DTO。
- 数据库真实异常仍向上返回错误。
- 存在 active 配置时，返回真实配置并正确脱敏 API key。

### 6.3 回归测试

- 保存 LLM 配置后，再次 `GET /config/llm` 返回真实配置。
- 保存 image model 配置后，再次 `GET /config/image-model` 返回真实配置。
- 已有 active 配置场景的响应结构保持兼容。
- API key 原文不在 GET 响应中泄露。

---

## 7. 验收标准

### 7.1 空库接口验收

在空数据库或无 active 模型配置的环境中执行：

```bash
curl -s http://localhost:8082/config/llm
curl -s http://localhost:8082/config/image-model
```

两个接口都必须满足：

- 返回业务 `code=0`。
- `data.api_key_masked=""`。
- `data.is_active=false`。
- 前端拦截器不弹 `failed to get config`。

### 7.2 保存后接口验收

用户在页面保存真实配置后：

- 再次 GET 对应接口返回业务 `code=0`。
- `data.is_active=true`。
- 返回保存后的 provider/model/base_url 等真实配置字段。
- API key 只返回 masked 值，不返回明文。

---

## 8. 设计结论

"未配置"应被建模为模型配置页的正常空状态，而不是接口错误。后端通过默认占位 DTO 承担空状态语义，前端可以继续使用统一的成功响应渲染表单。数据库不预置任何 API key，用户保存前系统只返回可编辑的空配置。
