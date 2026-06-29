# 弱密码容器 Env、采集进度与批量删除修复

## 背景

Redis 容器弱密码任务在采集到受控凭据记录后会直接进入服务端字典匹配。如果采集记录已经存在，任务不需要进入 AI 修复循环；AI 修复循环只用于配置文件缺失、字段未命中等采集失败场景。

## 变更

1. 容器 Env 优先采集
   - Agent 通过 `/proc/<pid>/cgroup` 提取 Docker 容器 ID。
   - 读取 `/var/lib/docker/containers/<container_id>/config.v2.json` 的 `Config.Env`。
   - 优先提取 password/passwd/pass/pwd/secret/token/auth/credential 等密码相关环境变量。
   - 若 Env 未产生凭据记录，再回退到配置文件、进程 cmdline、Docker Cmd 原始参数等既有逻辑。
   - 创建弱密码任务时会将候选应用缓存 PID 与资产采集的最新 PID 合并，避免容器重启或新增容器后仍使用旧 PID。

2. 采集进度字段
   - `WeakPassword.CollectCredentials` 的结果摘要增加 `source_path`、`field_name`、`source_paths`、`field_names`。
   - 任务详情页“采集进度”移除“说明”列，改为显示“采集路径”和“采集字段”。
   - 采集路径示例：`/etc/shadow`、`/var/lib/docker/containers/<container_id>/config.v2.json`。
   - 采集字段示例：`shadow.password`、`Env.REDIS_PASSWORD`、`docker_config.requirepass`。

3. 批量删除
   - 新增 `POST /api/v1/weak-password/tasks/batch-delete`。
   - 已完成、失败、未命中、已命中的任务可删除。
   - 运行中的任务会跳过并返回 skipped，避免前端误删。
   - 前端任务列表增加多选列和“批量删除”按钮。

## 验证要求

- 后端单测覆盖容器 Env 字段、采集进度路径/字段、批量删除跳过运行中任务。
- 真实环境验证需要使用真实 Aegis API、真实 Agent、真实 Docker Redis 容器和 Playwright 前端登录。
- 若 Redis 密码不在默认字典中，应创建自定义字典包含容器密码，再创建真实 Redis 弱密码任务验证命中。
