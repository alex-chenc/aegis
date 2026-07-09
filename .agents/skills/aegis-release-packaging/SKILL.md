---
name: aegis-release-packaging
description: 构建和验证 Aegis Linux AMD64 离线发布包，包括业务与基础 Docker 镜像、数据库初始化、启动脚本、环境模板及预置 Agent 的 MinIO 镜像。用于创建新版本发布包、检查发布产物或修复离线部署流程。
---

# Aegis 离线发布打包

以 `scripts/build_release_package.sh` 为发布流程的唯一实现来源。不要在 skill 中复制 Dockerfile、Compose、SQL 或启动脚本；需要改变产物时修改并验证该脚本。

## 目标与输入

产出 `release/aegis-<version>-linux-amd64-release.zip`。开始前确认：

- 目标版本；未明确时从当前设计文档、发布约定或用户输入确定，不沿用旧示例版本。
- 是否只生成结构，还是执行 Agent、镜像和 zip 的完整构建。
- Docker daemon、`gzip`、`zip`、构建工具和所需基础镜像是否可用。
- 现有同名目录或 zip 是否允许覆盖。

网络拉取、覆盖已有发布目录/zip、远程上传和启动部署栈都可能产生副作用。仅在请求已授权相应操作时执行；不要擅自设置 `FORCE=1`。

## 工作流

1. 读取 `scripts/build_release_package.sh`、`.env.example`、`docker-compose.yml`、`migrations/` 及受影响组件的构建文件。
2. 检查发布脚本是否仍与仓库一致：
   - Agent 构建包含所需 eBPF 对象和 Linux AMD64 二进制；
   - `migrations/*.sql` 按确定顺序汇入初始化 SQL；
   - 应用镜像、基础镜像及 MinIO Agent 制品都被导出；
   - Compose 的镜像名、依赖、健康检查、端口和环境变量与运行时配置一致；
   - `start.sh` 能加载镜像、准备 `.env`、确定外部地址并等待服务就绪。
3. 需要先审查生成内容时运行：

   ```bash
   GENERATE_ONLY=1 ./scripts/build_release_package.sh <version>
   ```

4. 需要完整发布包时运行：

   ```bash
   ./scripts/build_release_package.sh <version>
   ```

5. 验证结构、压缩包、镜像清单、初始化 SQL 和 Agent 制品。环境允许时，在隔离目录或测试主机执行部署冒烟。

如果脚本缺少必要能力，做最小修改并在同一版本上重新验证；不要绕过脚本手工拼装一个无法复现的发布包。

## 产物契约

发布目录至少应包含：

```text
<version>/
  images/*.tar.gz
  build-context/aegis-agent-linux-amd64.tar.gz
  build-context/bpf/*.bpf.o
  backend/scripts/init.sql
  docker-compose.yml
  .env.example
  start.sh
  README.md
```

MinIO 中供安装接口读取的 Agent 对象名、Compose 中的镜像名和 API 生成的下载地址必须一致。不要用硬编码凭证；发布模板只能包含明确标注的占位值或安全默认策略。

## 验证

按可用环境逐层验证：

```bash
unzip -t release/aegis-<version>-linux-amd64-release.zip
gzip -t release/<version>/images/*.tar.gz
```

解压后检查文件可执行性、镜像归档可加载、迁移来源齐全且顺序稳定。若执行部署冒烟，至少确认：

```bash
docker compose ps
curl -fsS http://localhost:8082/health
curl -fsS http://localhost:8082/api/v1/agent/install.sh
curl -fsS "http://localhost:8082/api/v1/agent/download?os=linux&arch=amd64" -o /tmp/aegis-agent.tar.gz
test -s /tmp/aegis-agent.tar.gz
```

数据库检查应针对本版本新增或关键表，而不是依赖固定历史表名。

## 停止与报告

满足以下条件后停止：目标 zip 已生成且完整性检查通过；所需部署冒烟通过，或无法执行的环境依赖已明确说明。报告版本、产物路径、大小、执行过的验证、失败证据和剩余风险。
