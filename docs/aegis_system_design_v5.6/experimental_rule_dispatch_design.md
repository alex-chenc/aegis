# 实验性规则下发设计

## 背景

规则状态 `experimental` 表示规则处于观察期，可能存在误报，尚未正式转为 `active`。当前实现中存在两套行为：AI 收紧规则会立即广播给在线 Agent，但 Agent 重连或全量同步时 `GetActiveAndExperimental` 会过滤掉进入实验性不足 24 小时的规则。

## 新设计

实验性规则也要下发到 Agent。

状态语义调整为：

| 状态 | 是否下发 | 含义 |
| --- | --- | --- |
| `pending` | 否 | 等待审核或等待自动进入实验性 |
| `experimental` | 是 | 已下发试运行，仍处于误报观察阶段 |
| `active` | 是 | 正式启用 |
| `disabled` | 否 | 停用并从 Agent 删除 |

## 行为要求

1. 全量同步必须包含 `active` 和所有 `experimental` 规则，不再按 `activated_at` 做 24 小时过滤。
2. AI 规则更新设为 `experimental` 后继续立即广播给在线 Agent。
3. Agent 重连后也能通过全量同步拿到刚进入 `experimental` 的规则。
4. `experimental` 仍保留 7 天后自动晋升 `active` 的观察期语义。

## 验证

仓储层测试覆盖：

- 新进入 `experimental` 的规则会被 `GetActiveAndExperimental` 返回。
- `active` 会被返回。
- `pending` 和 `disabled` 不会被返回。
