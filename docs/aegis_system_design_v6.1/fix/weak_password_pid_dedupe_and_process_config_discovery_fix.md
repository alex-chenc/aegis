# 弱密码模块 PID 去重与进程配置发现修复

## Bug description and symptoms

1. 应用资产页同一主机上会重复显示同一运行应用，且页面未展示应用关联 PID，运维人员无法判断重复记录是否来自同一进程。
2. 弱密码检测第一轮配置文件路径不存在或未采集到路径时，后续修复过度依赖 LLM 选择辅助工具，缺少稳定的第二轮进程上下文发现。
3. 容器化应用的配置文件位于容器根目录内时，检测流程没有优先使用 `/proc/<pid>/cgroup` 判断容器身份，再通过 `/proc/<pid>/root` 定位容器内配置文件。

## Reproduction steps

1. 资产采集时让 LLM 在同一主机同一 PID 上输出多条应用记录，或输出同名但不同 PID 的应用实例。
2. 打开前端“主机资产 / 应用资产”，可看到重复应用且无 PID 列。
3. 创建 Redis 弱密码候选，初始路径指向不存在的配置文件，但候选资产包含 Redis 进程 PID。
4. 运行弱密码检测，第一轮 `WeakPassword.CollectCredentials` 返回 `file_not_found` 后，流程可能直接失败或依赖 LLM 修复，不能稳定发现容器内 `/etc/redis/redis.conf`。

## Root cause analysis

- `deduplicateApplications` 只按应用 `name` 去重，无法保证同一 PID 只属于一个应用资产，也会把同名不同 PID 的应用实例误合并。
- `generateAppFingerprint` 未纳入 PID；同一 PID 因安装路径、端口或 LLM 输出差异变化会生成多个 fingerprint，从而绕过 `UNIQUE(host_id, fingerprint)`。
- `ApplicationAsset` 前端类型缺少 `related_pids`，表格和详情没有展示 PID。
- 弱密码修复环路只在有 retryable errors 时调用 AI 修复；当候选路径为空或第一轮未定位配置文件时，没有确定性地读取 `/proc/<pid>/cgroup`、`cmdline` 和容器根目录。

## Fix design

- 应用资产分析：
  - 先规范化并排序 `related_pids`。
  - 有 PID 的应用按 PID 重叠去重：同一 PID 只保留/合并到一个应用资产；同名但 PID 不同的实例保留为不同资产。
  - 有 PID 的应用 fingerprint 使用 `host_id + related_pids`，保证同一 PID 集合重复采集时 upsert。
- 前端应用资产：
  - `ApplicationAsset` 增加 `related_pids`。
  - 表格和详情抽屉展示 PID。
- 弱密码检测：
  - Agent `WeakPassword.ProcessConfigHints` 增加 cgroup/container/cmdline 配置候选输出。
  - 容器 ID 判断参考 OpenTelemetry、cAdvisor 等开源项目常见 cgroup 命名：Docker、containerd、CRI-O、Podman/libpod 以及 systemd scope 中的 12-64 位 hex ID。
  - 如果检测到容器 ID，使用 `/proc/<pid>/root` 对常见配置路径、cmdline 路径和受限目录候选做非递归探测，返回容器内源路径。
  - 如果不是容器，则从 `/proc/<pid>/cmdline`、cwd 和打开文件中推断配置文件路径。
  - api-server 在第一轮未采集到记录后，先执行一次确定性的 `WeakPassword.ProcessConfigHints` 修复；发现新路径后更新采集计划并重试，再保留原有 AI 修复作为后备。

## Verification steps

- Go 单元测试：
  - `agent/internal/weakpass` 容器 ID 解析、cmdline 路径提取、容器 root 配置候选。
  - `api-server/internal/service` 应用 PID 去重、fingerprint、弱密码第二轮进程 hint 修复并命中字典。
- 前端测试：
  - Playwright mock 弱密码流程断言能看到 PID、容器内配置路径和弱密码命中。
- 构建验证：
  - `cd agent && go test ./internal/weakpass`
  - `cd api-server && go test ./internal/service`
  - `cd frontend && npm run test -- weakPassword`
  - `cd frontend && npx playwright test e2e/weak-password.spec.ts`

## Affected components

- `agent/internal/weakpass`
- `api-server/internal/service`
- `frontend/src/views/hosts/Assets/Applications.vue`
- `frontend/src/api/assets.ts`
- `frontend/e2e/weak-password.spec.ts`

## Risk and rollback plan

- 风险：PID fingerprint 会让同名不同 PID 的实例分开展示，历史数据仍按旧 fingerprint 保留，需等待后续采集 upsert 新记录。
- 风险：容器配置发现只做白名单路径和非递归目录扫描，不会覆盖所有自定义目录；保留 cmdline 和 AI 修复作为后备。
- 回滚：恢复旧 fingerprint 和 `deduplicateApplications`；关闭确定性 `ProcessConfigHints` 分支后，系统回到原 AI 修复流程。
