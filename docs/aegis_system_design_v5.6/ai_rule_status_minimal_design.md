# AI规则状态最小化设计

## 问题

AI规则更新现在有两套模式：

- `suggest`：仅建议，需要人工审核。
- `auto`：自动更新规则。

当前混乱点是 `experimental` 同时被用来表达“试运行”和“未正式启用/可能不下发”。用户看到“实验性”时无法判断规则到底有没有下发到 Agent。

## 最小设计原则

`status` 只回答一个核心问题：这条规则是否参与检测下发。

不要再出现“实验性但禁用”或“实验性但不下发”的组合。

## 状态语义

| 状态 | 中文 | 是否下发 Agent | 含义 |
| --- | --- | --- | --- |
| `pending` | 待审核 | 否 | AI建议或人工导入后等待确认 |
| `experimental` | 实验性 | 是 | 已下发试运行，可能误报，处于观察期 |
| `active` | 已激活 | 是 | 正式启用规则 |
| `disabled` | 已禁用 | 否 | 停用规则，并从 Agent 删除 |

核心约束：

- `experimental` 必须下发。
- `pending` 必须不下发。
- `disabled` 必须不下发。
- `active` 必须下发。

## 两种AI模式

### 仅建议模式 `suggest`

AI只生成建议，不改变线上检测规则。

行为：

1. 新规则建议：创建 `pending` 规则，不下发。
2. 旧规则优化建议：不直接覆盖原规则；创建一条 `pending` 建议规则，使用 `parent_rule_id` 指向原规则。
3. 人工审核通过后：
   - 默认进入 `experimental` 并下发。
   - 观察期结束后再自动转 `active`。
4. 人工拒绝或禁用后：状态为 `disabled`，不下发。

仅建议模式下不再自动把 `pending` 转成 `experimental`。如果需要自动下发，应使用 `auto` 模式。

### 自动模式 `auto`

AI可以直接更新线上规则，但仍进入观察期。

行为：

1. 新规则：创建为 `experimental`，立即下发。
2. 旧规则优化：直接更新原规则内容，状态设为 `experimental`，立即下发。
3. 观察期结束后自动转 `active`。
4. 用户禁用后：状态为 `disabled`，从 Agent 删除。

## 下发查询

Agent全量同步只取：

```text
status IN ('experimental', 'active')
```

增量广播规则：

| 状态变化 | 广播动作 |
| --- | --- |
| `pending -> experimental` | `add/update` |
| `experimental -> active` | `update` |
| `active/experimental -> disabled` | `delete` |
| `pending -> disabled` | 不需要下发 |

## 前端展示

规则状态文案保持简洁：

- 待审核：未下发
- 实验性：已下发试运行
- 已激活：已正式下发
- 已禁用：未下发

建议在状态列 tooltip 中直接显示是否下发，避免用户从状态猜测。

## 迁移处理

已有数据按新语义解释：

- `experimental`：视为已下发试运行。
- `pending`：视为未下发建议。
- `disabled`：视为停用不下发。

如果历史上存在“仅建议但被标记为 experimental”的规则，应改回 `pending`，否则它会被下发。

## 实现落点

- `suggest` 模式下，AI 对已有规则的优化创建新的 `pending` 建议规则，并通过 `parent_rule_id` 指向原规则；原规则不被覆盖。
- `auto` 模式下，AI 新生成或优化的规则进入 `experimental`，并按现有广播路径下发。
- `pending` 不再按 24 小时定时任务自动转为 `experimental`。
- 人工启用 `pending`/`disabled` 规则时先进入 `experimental`；再次启用 `experimental` 时才转为 `active`。
- 前端配置面板隐藏旧的“24小时自动实验性”开关，规则状态列通过悬浮提示说明是否下发到 Agent。
