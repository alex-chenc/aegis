# 弱密码容器应用路径与资产标签修复设计 V6.1

## 背景

在“弱密码检查 - Kafka”的采集进度中，进程配置线索只显示了 `/proc/<pid>/root`，后续候选配置仍显示为 `/etc/kafka/...`。Agent 实际读取时会优先通过容器 PID 拼接 `/proc/<pid>/root/<容器内路径>`，但 API 汇总和页面展示没有表达真实读取路径，容易误判为仍在宿主机路径下检测。

同时，智能资产采集的应用资产缺少“容器应用 / 主机应用”标签，资产分析提示词也没有把 `/proc/<pid>/cgroup` 原文交给大模型，导致后续弱密码检测需要重复推断容器属性。

## 目标

1. 应用资产保存显式容器元数据：`is_container`、`container_id`、`container_runtime`。
2. 智能资产采集页面应用资产表格展示“容器应用 / 主机应用”标签。
3. 资产分析提示词包含 `/proc/<pid>/cgroup`，并要求模型返回 `is_container`。
4. 弱密码候选应用、采集计划和任务进度显示是否为容器应用。
5. 容器应用弱密码检测固定走容器逻辑：先采集容器环境变量，再按 `/proc/<pid>/root` 拼接配置路径读取。
6. 采集进度表格固定显示高度，长路径/字段截断，鼠标悬浮展示全量。

## 设计

### 资产采集

- Agent 进程资产新增 `cgroup` 字段，保存 `/proc/<pid>/cgroup` 的非空行。
- API `ProcessAsset` 接收 `cgroup` 字段，并在资产分析 prompt 中展示。
- Prompt 写入 Agent 容器判断规则：
  - cgroup 中出现 Docker/containerd/cri-o/libpod/podman/kubepods scope 或 64 位容器 ID 时，认为是容器进程。
  - 非容器进程通常是 `0::/` 或普通 system.slice/user.slice，且没有容器 ID。
- LLM 输出的应用对象增加：
  - `is_container`
  - `container_id`
  - `container_runtime`
- 保存应用资产时将模型返回和进程快照证据合并，以进程 cgroup 解析结果为准。

### 弱密码检测

- 弱密码候选应用继承应用资产的容器字段，DTO 返回 `is_container`、`container_id`、`container_runtime`。
- 采集计划下发给 Agent 时包含同样的容器字段。
- Agent 对 `is_container=true` 的应用：
  - 先读取 `/var/lib/docker/containers/<容器ID>/config.v2.json` 中的 `Config.Env`，提取疑似密码变量。
  - 如未命中，再将配置路径按 `/proc/<pid>/root/<path>` 拼接读取。
  - 不再回落到宿主机同名路径，避免容器路径和宿主机路径混淆。
- 采集结果和错误中的 `source_path` 使用真实读取路径，例如 `/proc/6937/root/etc/kafka/server.properties`。
- 采集计划、AI 修复计划中的 `new_paths` 仍保持应用视角路径，例如 `/etc/kafka/server.properties`，避免二次拼接。

### 前端

- 弱密码任务详情“采集进度”表格设置固定高度，路径和字段单元格最多展示多行，悬浮显示全量。
- 智能资产采集“应用资产”表格新增“标签”列：
  - 容器应用：绿色小标签
  - 主机应用：灰色小标签
- 弱密码“应用资产分析”表格新增同样标签，便于检测前确认容器属性。

## 验证

1. 后端单元测试：
   - Agent 容器路径读取候选不回落宿主机。
   - API 资产分析 prompt 包含 cgroup 和容器输出字段。
   - 弱密码计划从资产继承容器字段。
2. 构建与重启：
   - `cd agent && go test ./internal/weakpass ./internal/assets -count=1`
   - `cd api-server && go test ./internal/service -count=1`
   - `cd api-server && make build`
   - `cd frontend && npm run build`
   - 重启 agent、api-server、frontend。
3. 真实 Playwright：
   - 使用 `admin/Admin@123` 登录真实系统。
   - 触发真实 Kafka 弱密码检测。
   - 断言页面采集路径出现 `/proc/<pid>/root/etc/kafka/...`。
   - 断言应用资产表存在“容器应用 / 主机应用”标签。
