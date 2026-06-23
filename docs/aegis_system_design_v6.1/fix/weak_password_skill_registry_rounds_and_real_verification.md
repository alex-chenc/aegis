# 弱密码应用 Skill、轮数与真实验证设计

## 背景

本次修复覆盖弱密码模块的 8 个问题：应用资产同主机去重、容器进程配置发现、固定应用弱密码 skill、通用弱密码 skill、OpenSSH `/etc/shadow`、检查轮数、采集进度分页、AI 字典生成以及真实环境验证。

## 上游参考

- `chaitin/veinmind-tools/plugins/go/veinmind-weakpass`：作为容器/镜像弱口令扫描器，README 明确支持 `ssh/mysql/redis/tomcat/ftp`；代码中 Redis 读取 `requirepass`，SSH 读取 `/etc/shadow`，Tomcat 读取 `tomcat-users.xml`，MySQL 解析 `mysql.user` 存储文件，FTP 覆盖 `vsftpd` 与 `proftpd`。
- `CISOfy/lynis`：系统认证审计覆盖 `/etc/passwd`、`/etc/shadow`、PAM、OpenSSH 检查；`AUTH-9229` 会读取 `/etc/passwd` 和 `/etc/shadow` 判断 hash 方法与 rounds，`AUTH-9283` 检查空密码账号。
- `getarcaneapp/arcane/backend/pkg/dockerutil/cgroup_utils.go`：cgroup v1 识别 `/docker/<64hex>`，cgroup v2 识别 `docker-<64hex>.scope`，并按 cgroup、mountinfo、hostname 顺序获取当前容器 ID。本次 Aegis 针对任意进程读取 `/proc/<pid>/cgroup`，兼容 docker/containerd/cri-o/podman/libpod 与裸 64 hex 片段。

## 应用弱密码 Skill Registry

后端引入固定 `WeakPasswordSkill` registry，由应用类型选择确定的路径、解析器和匹配策略；未命中固定应用时走通用 skill。

| 应用 | 配置/凭据路径 | 采集方式 | 匹配方式 |
| --- | --- | --- | --- |
| Redis | `/etc/redis/redis.conf`、`/etc/redis.conf`、`/usr/local/etc/redis/redis.conf`、`/data/redis.conf` | `line_key_value` 读取 `requirepass`、`masterauth` | 字典明文精确匹配 |
| OpenSSH/sshd | `/etc/shadow` | `shadow` 解析账号、hash、salt、算法 | 服务端 verifier 校验 shadow hash，空密码直接匹配 |
| MySQL/MariaDB | `/etc/mysql/my.cnf`、`/etc/my.cnf`、`/root/.my.cnf`、`/var/lib/mysql/mysql/user.MYD`、`/var/lib/mysql/mysql.ibd` | 先解析配置中的客户端账号密码，存储文件保留为后续专用 parser 路线 | 明文或 MySQL hash verifier |
| Tomcat | `/usr/local/tomcat/conf/tomcat-users.xml`、`/etc/tomcat*/tomcat-users.xml` | `tomcat_users_xml` 解析 `<user username password>` | 明文精确匹配 |
| FTP | `/etc/vsftpd/virtual_users.db`、`/etc/proftpd/passwd`、`/etc/proftpd/ftpd.passwd`、`/etc/shadow` | 明文/类 shadow 解析 | 明文或 shadow verifier |
| Nginx/Apache | `.htpasswd` 常见路径 | `htpasswd` | 服务端 verifier |
| Generic | 应用资产配置路径、启动路径、进程提示路径 | `line_key_value/yaml/json/properties` 常见 `user/password/token` 字段 | 明文精确匹配 |

## 采集与容器路径

1. 第一轮按固定 skill 路径和应用资产路径执行 `WeakPassword.CollectCredentials`。
2. 若第一轮没有凭据，且应用资产有 PID，则调用 `WeakPassword.ProcessConfigHints`。
3. Agent 读取 `/proc/<pid>/cgroup`。若存在容器 ID，则用 `/proc/<pid>/root` 作为容器根目录验证固定路径与 cmdline/open fd 路径。
4. 若不是容器，则读取 `/proc/<pid>/cmdline`、`cwd`、打开文件，提取配置路径再重试。
5. 检测轮数由用户设置，最小 10、最大 50；所有受控 Agent 工具调用共享这个上限。

## 前端交互

- “检查弱密码”抽屉新增检测轮数输入，范围 10-50，默认 10。
- 任务详情把“采集失败”改为“采集进度”，数据来自 Agent 工具调用记录。
- 采集进度固定每页 10 条，分页 `pager-count=10`，超过 10 条显示分页。

## 测试计划

- Agent 单测：容器 cgroup 识别、容器 `/proc/<pid>/root` 路径发现、Tomcat XML、shadow 解析。
- API 单测：skill registry 固定路径、检测轮数归一化、进度分页、OpenSSH shadow hash 匹配、应用资产同主机去重、AI 字典 fallback 非顺序化。
- Frontend/e2e：检查抽屉轮数字段、创建任务 payload、采集进度文案与分页。
- 真实验证：构建服务，重装 Agent，启动一个弱口令 Redis 容器，确认应用资产采集出 Redis，再用 Playwright 登录 `admin/Admin@123` 跑到弱密码命中。
