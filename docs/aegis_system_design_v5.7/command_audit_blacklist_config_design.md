# V5.7 命令审计黑名单配置设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 功能概述

在系统配置模块新增"命令审计配置"页面，允许管理员管理脚本命令黑名单。黑名单规则在脚本生成、下发、执行各阶段被 `ScriptAuditService` 和 `BlacklistChecker` 引用。

---

## 2. 数据库设计

### 2.1 command_audit_rules 表

```sql
CREATE TABLE command_audit_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(200) NOT NULL,
    description     TEXT,
    rule_type       VARCHAR(20) NOT NULL DEFAULT 'hard_block',
    match_type      VARCHAR(20) NOT NULL DEFAULT 'regex',
    pattern         TEXT NOT NULL,
    category        VARCHAR(50) NOT NULL DEFAULT 'system',
    severity        VARCHAR(20) NOT NULL DEFAULT 'high',
    applies_to      JSONB NOT NULL DEFAULT '["all"]',
    is_preset       BOOLEAN NOT NULL DEFAULT false,
    is_enabled      BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_command_audit_rules_category ON command_audit_rules(category);
CREATE INDEX idx_command_audit_rules_enabled ON command_audit_rules(is_enabled);
CREATE INDEX idx_command_audit_rules_severity ON command_audit_rules(severity);
```

字段说明：
- `rule_type`: `hard_block`（命中即拦截）/ `soft_warn`（记录警告，继续AI审计）
- `match_type`: `exact`（全命令精确匹配）/ `regex`（正则表达式匹配）
- `category`: `filesystem` / `permission` / `network` / `system` / `privilege`
- `severity`: `critical` / `high` / `medium`
- `applies_to`: JSON数组，`["all"]` 或 `["baseline", "vulnerability", "poc", "self_healing"]`
- `is_preset`: 预置规则不可删除，但可启停

### 2.2 system_configs 表

```sql
CREATE TABLE system_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key      VARCHAR(200) NOT NULL UNIQUE,
    config_value    JSONB NOT NULL,
    description     TEXT,
    category        VARCHAR(50) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_system_configs_category ON system_configs(category);
```

命令审计相关配置：

| config_key | config_value示例 | 说明 |
|:---|:---|:---|
| `command_audit.settings` | `{"blacklist_enabled": true, "ai_enabled": true, "max_retry": 3}` | 审计策略开关 |

---

## 3. API设计

### 3.1 命令审计规则API

| 方法 | 路径 | 说明 |
|:---|:---|:---|
| GET | `/api/v1/settings/command-audit/rules` | 获取规则列表（分页、筛选） |
| POST | `/api/v1/settings/command-audit/rules` | 新增规则 |
| PUT | `/api/v1/settings/command-audit/rules/:id` | 编辑规则 |
| DELETE | `/api/v1/settings/command-audit/rules/:id` | 删除规则（预置规则不可删） |
| PUT | `/api/v1/settings/command-audit/rules/:id/toggle` | 启停规则 |
| POST | `/api/v1/settings/command-audit/rules/batch-toggle` | 批量启停 |
| POST | `/api/v1/settings/command-audit/rules/test` | 测试规则匹配 |
| GET | `/api/v1/settings/command-audit/settings` | 获取审计策略配置 |
| PUT | `/api/v1/settings/command-audit/settings` | 更新审计策略配置 |

### 3.2 请求/响应示例

**新增规则**:
```json
{
  "name": "禁止curl管道执行",
  "description": "禁止通过curl下载脚本并直接通过管道执行",
  "rule_type": "hard_block",
  "match_type": "regex",
  "pattern": "(curl|wget).*\\|\\s*(bash|sh|zsh)",
  "category": "network",
  "severity": "critical",
  "applies_to": ["all"]
}
```

**测试规则**:
```json
{
  "match_type": "regex",
  "pattern": "(curl|wget).*\\|\\s*(bash|sh|zsh)",
  "test_content": "#!/bin/bash\ncurl -sSL https://example.com/setup.sh | bash\napt update"
}
```

**测试响应**:
```json
{
  "matched": true,
  "matches": [
    {
      "line_number": 2,
      "matched_text": "curl -sSL https://example.com/setup.sh | bash"
    }
  ]
}
```

---

## 4. 前端页面设计

### 4.1 页面布局

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  命令审计配置                                                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─── 审计策略 ──────────────────────────────────────────────────────────┐  │
│  │  ┌──────────────────────┐  ┌──────────────────────┐                  │  │
│  │  │ B  黑名单审计     [ON]│  │ AI AI 审计        [ON]│                  │  │
│  │  │    基于预置规则的确定 │  │    基于大模型的上下文 │                  │  │
│  │  │    性检查，命中即拦截 │  │    风险分析，检测隐蔽 │                  │  │
│  │  │                      │  │    威胁  [未配置LLM]  │                  │  │
│  │  └──────────────────────┘  └──────────────────────┘                  │  │
│  │  ┌──────────────────────┐  ┌──────────────────────┐                  │  │
│  │  │ P  下发前校验     [ON]│  │ A  Agent 侧校验   [ON]│                  │  │
│  │  │    脚本从API Server  │  │    Agent 执行前的最后 │                  │  │
│  │  │    下发前再次校验黑名 │  │    一道防线           │                  │  │
│  │  │    单                 │  │                      │                  │  │
│  │  └──────────────────────┘  └──────────────────────┘                  │  │
│  │  最大重试次数: [3]  AI审计失败后重新生成脚本的最大尝试次数              │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌─── 黑名单规则 ────────────────────────────────────────────────────────┐  │
│  │  [+ 新增规则]    搜索: [_________]                   │  │
│  │  筛选: [分类 ▾] [严重等级 ▾] [匹配类型 ▾] [状态 ▾]                  │  │
│  │                                                                        │  │
│  │  名称            │ 分类     │ 等级    │ 类型  │ 匹配模式         │状态│  │
│  │  ────────────────────────────────────────────────────────────────────│  │
│  │  递归删除根目录   │ 文件系统 │ critical│ regex │ rm\s+(-r...)\s+ │ ON│  │
│  │  格式化磁盘       │ 文件系统 │ critical│ regex │ mkfs\.          │ ON│  │
│  │  Fork炸弹         │ 系统    │ critical│ exact │ :(){ :\|:& };:  │ ON│  │
│  │  管道执行远程脚本 │ 网络    │ critical│ regex │ (curl|wget)...  │ ON│  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 5. 预置规则集

完整预置规则集（25条）：

| # | 名称 | match_type | pattern | category | severity |
|:--|:-----|:-----------|:--------|:---------|:---------|
| 1 | 递归删除根目录 | regex | `rm\s+(-[a-zA-Z]*r[a-zA-Z]*f\|-[a-zA-Z]*f[a-zA-Z]*r)\s+/` | filesystem | critical |
| 2 | 删除根目录 | regex | `rm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?/\s*$` | filesystem | critical |
| 3 | 格式化磁盘 | regex | `mkfs\.` | filesystem | critical |
| 4 | 覆盖磁盘设备 | regex | `dd\s+if=/dev/(zero\|random\|urandom)` | filesystem | critical |
| 5 | 写入块设备 | regex | `(echo\|printf\|cat\|dd).*>\s*/dev/[sh]d[a-z]` | filesystem | critical |
| 6 | Fork炸弹 | exact | `:(){ :\|:& };:` | system | critical |
| 7 | 递归777权限 | regex | `chmod\s+(-R\s+)?777\s+/` | permission | high |
| 8 | 递归修改属主 | regex | `chown\s+(-R\s+)?root:root\s+/` | permission | high |
| 9 | curl管道执行 | regex | `curl\s+.*\|\s*(ba)?sh` | network | critical |
| 10 | wget管道执行 | regex | `wget\s+.*-O\s*-\s*\|\s*(ba)?sh` | network | critical |
| 11 | Netcat反弹Shell | regex | `nc\s+.*-e\s*/bin/(ba)?sh` | network | critical |
| 12 | Bash反弹Shell | regex | `bash\s+-i\s+>&\s+/dev/tcp/` | network | critical |
| 13 | Python Socket反弹 | regex | `python[23]?\s+.*import\s+socket.*connect` | network | high |
| 14 | Perl Socket反弹 | regex | `perl\s+.*socket.*connect` | network | high |
| 15 | 停止防火墙 | regex | `systemctl\s+(stop\|disable\|mask)\s+(firewalld\|iptables\|ufw)` | system | high |
| 16 | 停止SSH服务 | regex | `systemctl\s+(stop\|disable\|mask)\s+sshd` | system | high |
| 17 | 删除root用户 | regex | `userdel\s+(-r\s+)?root` | system | critical |
| 18 | 修改shadow文件 | regex | `(echo\|printf\|tee\|cat.*>)\s*.*>\s*/etc/shadow` | system | critical |
| 19 | 修改passwd文件 | regex | `(echo\|printf\|tee\|cat.*>)\s*.*>\s*/etc/passwd` | system | critical |
| 20 | 添加sudoer | regex | `(echo\|printf).*>>\s*/etc/sudoers` | privilege | high |
| 21 | 清空系统日志 | regex | `(echo\s*>\|truncate\s+-s\s+0)\s*/var/log/` | system | high |
| 22 | 禁用SELinux | regex | `setenforce\s+0` | system | high |

预置规则特点：
- 不可删除（`is_preset=true`），但可启停
- 系统首次初始化时自动创建
- 后续版本可通过迁移脚本追加新预置规则

---

## 6. 安全考虑

| 风险 | 缓解措施 |
|:---|:---|
| 管理员被钓鱼添加放行规则 | 预置规则不可删除；操作记录审计日志 |
| 正则ReDoS | 正则预编译+10ms超时+1000字符长度限制 |
| 规则过多影响性能 | 按category索引，按脚本类型过滤，hard_block短路返回 |
| 多实例配置不一致 | 规则存储在数据库，所有实例共享 |
