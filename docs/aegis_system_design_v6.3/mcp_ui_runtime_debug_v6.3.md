# MCP 聚合管控 UI 运行回归记录（V6.3）

## 2026-09-16：首屏 500 与重复错误提示

MCP 聚合管控页面首屏并发请求 `/mcp-platform/servers`、`/overview`、`/onboarding-jobs`、`/catalogs` 时，api-server 在角色中间件查询 `role_permissions` 处返回 PostgreSQL `42P01 relation does not exist`，四个请求均为 HTTP 500。前端 Axios 拦截器会对每个失败请求显示一次错误提示，因此用户看到重复的“服务器内部错误”。

根因是启动迁移重构时从 `db.go` 的 GORM AutoMigrate 清单移除了仍被权限中间件依赖的 `RolePermission`；Compose 挂载的根 `migrations/` 未包含旧 api-server `008_detection_package_v5.8_fix.sql`，所以持久卷和新卷都可能缺表。MCP 039 OPA 表已存在，未参与该故障调用链。

修复将 `RolePermission` 保留在独立的 `autoMigrateModels()` 清单中，其他迁移所有模型继续由正式 SQL migrations 管理；并增加 `TestAutoMigrateModelsIncludeLegacyRolePermissionSchema` 防回归。api-server 已完成镜像重建并强制重建，启动日志显示 GORM 创建 `role_permissions`，数据库核对返回 `role_permissions` 且当前 0 行，服务为 healthy。重建后经 frontend Nginx 代理访问四个 URL 的无凭证冒烟均按认证契约返回 401（不再是 500），Nginx 时间 `2026-09-16T09:35:11Z`；健康检查返回 200。当前执行环境没有浏览器调试端口，未提取或伪造用户 Bearer token，因此受保护的真实会话 200 需用户刷新页面后确认。
