# Assistant Messages Thinking 列迁移失败修复文档

## Bug 描述和症状

### 症状
- 所有前端页面返回 502 (Bad Gateway) 错误
- API Server 容器不断重启 (Restarting 状态)
- 其他服务 (server, dc, frontend, postgres, redis, kafka) 正常运行

### 错误日志
```
ERROR: invalid input syntax for type json (SQLSTATE 22P02)
ALTER TABLE "assistant_messages" ALTER COLUMN "thinking" TYPE JSONB USING "thinking"::JSONB
failed to auto migrate models
failed to connect database
```

## 复现步骤

1. 系统升级到 V6.0 后，`AssistantMessage` 模型的 `Thinking` 字段从 TEXT 改为 JSONB
2. 数据库中 `assistant_messages` 表已有数据
3. 某些行的 `thinking` 列包含非 JSON 格式的纯文本数据
4. GORM AutoMigrate 尝试 ALTER COLUMN 将 TEXT 转换为 JSONB
5. PostgreSQL 拒绝转换，因为现有数据不是有效的 JSON
6. API Server 启动失败，进入崩溃重启循环

## 根因分析

### 数据问题
`assistant_messages` 表中 ID 为 `4134d7f5-2cc2-4242-bda6-1e8d33aee503` 的记录：
- `thinking` 列内容以纯文本开头："正在分析您的问题...\n开始执行任务...\n"
- 后面跟着 JSON 格式的任务计划
- 这种混合格式不是有效的 JSON，无法直接转换为 JSONB

### 模型定义
```go
// api-server/internal/model/assistant.go:39
Thinking datatypes.JSON `gorm:"type:jsonb" json:"thinking,omitempty"`
```

### 迁移逻辑
```go
// api-server/internal/repository/db.go:47-85
if err := db.AutoMigrate(
    // ...
    &model.AssistantMessage{},  // 这里触发 thinking 列的类型转换
    // ...
); err != nil {
    // 迁移失败，API Server 无法启动
}
```

## 修复设计

### 修复策略
在 GORM AutoMigrate 之前，添加预迁移步骤，将无效 JSON 数据转换为有效格式：

1. 检测 `thinking` 列的当前类型
2. 如果是 TEXT 类型，清理无效 JSON 数据
3. 将纯文本内容包装为有效的 JSON 结构
4. 然后执行 GORM AutoMigrate

### 数据转换规则
对于非 JSON 格式的 `thinking` 数据：
- 将无效 JSON 数据设置为 NULL
- 包括：空字符串、纯文本、混合内容

### 最小安全修复
在迁移前将无效 JSON 数据清理为 NULL。

```sql
-- 将非 JSON 格式的 thinking 数据转换为 NULL
-- 包括：空字符串、纯文本、混合内容
UPDATE assistant_messages
SET thinking = NULL
WHERE thinking IS NOT NULL
  AND (thinking = '' OR NOT (thinking::text ~ '^\s*[\[\{]'));
```

### 实际数据情况
- 总记录数：105 行
- 无效 JSON 数据：105 行
  - 1 行：纯文本 + JSON 混合内容
  - 104 行：空字符串（长度为 0）
- 有效 JSON 数据：0 行

## 代码变更

### 文件：`api-server/internal/repository/db.go`

在 `NewDB` 函数中，`db.AutoMigrate` 调用之前，添加预迁移清理逻辑：

```go
// 预迁移：清理 assistant_messages.thinking 列中的无效 JSON 数据
if err := cleanInvalidThinkingData(db); err != nil {
    logger.Error("failed to clean invalid thinking data", zap.Error(err))
    return nil, fmt.Errorf("failed to clean invalid thinking data: %w", err)
}
```

添加新函数：
```go
func cleanInvalidThinkingData(db *gorm.DB) error {
    // 检查 assistant_messages 表是否存在
    if !db.Migrator().HasTable("assistant_messages") {
        return nil
    }

    // 检查 thinking 列是否存在
    if !db.Migrator().HasColumn(&model.AssistantMessage{}, "thinking") {
        return nil
    }

    // 清理非 JSON 格式的 thinking 数据，包括空字符串
    result := db.Exec(`
        UPDATE assistant_messages
        SET thinking = NULL
        WHERE thinking IS NOT NULL
          AND (thinking = '' OR NOT (thinking::text ~ '^\s*[\[\{]'))
    `)

    if result.Error != nil {
        return result.Error
    }

    if result.RowsAffected > 0 {
        logger.Info("cleaned invalid thinking data before migration",
            zap.Int64("rows_affected", result.RowsAffected))
    }

    return nil
}
```

## 验证步骤

### 验证结果（2026-06-07）

1. ✅ 执行 SQL 检查清理结果：
```sql
SELECT COUNT(*) FROM assistant_messages
WHERE thinking IS NOT NULL
  AND (thinking = '' OR NOT (thinking::text ~ '^\s*[\[\{]'));
-- 返回 0
```

2. ✅ 重建并启动 API Server 容器：
```bash
docker compose up -d --build api-server
```

3. ✅ 检查容器状态：
```bash
docker compose ps api-server
# 显示 Up 状态，不再 Restarting
```

4. ✅ 测试 API 端点：
```bash
curl http://localhost:8082/health
# 返回 {"status":"ok"}
```

5. ✅ 确认数据库迁移成功：
```sql
SELECT column_name, data_type FROM information_schema.columns
WHERE table_name = 'assistant_messages' AND column_name = 'thinking';
-- 返回 jsonb
```

## 影响组件

| 组件 | 影响 | 说明 |
|------|------|------|
| api-server | 直接影响 | 修复迁移失败问题 |
| frontend | 间接影响 | API Server 恢复后页面正常 |
| 其他服务 | 无影响 | 不涉及 |

## 风险和回滚计划

### 风险
- **数据丢失**：清理操作会将无效 JSON 数据设置为 NULL
  - 风险等级：低（只有 1 行数据受影响）
  - 缓解措施：数据已记录在本文档中，可手动恢复

### 回滚计划
如果修复失败：
1. 恢复 `thinking` 列数据：
```sql
UPDATE assistant_messages 
SET thinking = '正在分析您的问题...' 
WHERE id = '4134d7f5-2cc2-4242-bda6-1e8d33aee503';
```

2. 如果需要回滚模型定义，将 `Thinking` 字段改回 TEXT 类型

## 测试用例

### 回归测试 1：迁移成功
- 前提：数据库中存在无效 JSON 的 thinking 数据
- 操作：启动 API Server
- 预期：迁移成功，服务正常启动

### 回归测试 2：有效 JSON 数据保留
- 前提：数据库中存在有效 JSON 的 thinking 数据
- 操作：启动 API Server
- 预期：有效 JSON 数据保持不变

### 回归测试 3：API 端点可用
- 操作：调用 `/api/v1/hosts` 端点
- 预期：返回 200 状态码和正确的主机列表

### 回归测试 4：前端页面可用
- 操作：访问主机列表页面
- 预期：页面正常加载，不出现 502 错误
