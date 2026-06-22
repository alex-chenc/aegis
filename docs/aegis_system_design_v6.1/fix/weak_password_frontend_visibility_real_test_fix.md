# 弱密码检测前端可见性与真实测试修复

## Bug 描述

用户在真实前端页面看不到 V6.1 智能弱密码检测模块。进一步真实测试发现：

- 8081 前端容器仍在加载旧静态资源包，未包含新路由入口。
- 使用真实账号 `admin` 登录后，弱密码 API 在旧 api-server 容器中返回 404。
- 新后端部署后，默认弱密码字典元数据存在，但 entries 未落库，`entry_count` 为 0。

## 复现步骤

1. 访问 `http://127.0.0.1:8081/`。
2. 使用 `admin/Admin@123` 登录。
3. 访问 `/risk/weak-password` 或从侧边栏查找弱密码入口。
4. 点击“一键分析资产应用”或进入字典页。

## 根因分析

- 前端容器未重建，Nginx 仍提供旧 `index-*.js` 静态资源。
- api-server 容器运行旧二进制，未包含 `/api/v1/weak-password/*` 路由。
- `WeakPasswordDictionaryEntry` GORM 模型未声明 `(dictionary_id, candidate_hash)` 唯一索引，AutoMigrate 无法为已有表创建 upsert 冲突目标，默认字典 entries 无法可靠写入。

## 修复设计

- 将前端导航和页面标题改为“智能弱密码检测”，增强入口可见性。
- 增加真实 Playwright 测试 `frontend/e2e/weak-password-real.spec.ts`，通过 `PLAYWRIGHT_REAL=1` 连接真实 8081/8082。
- 在 `WeakPasswordDictionaryEntry` 模型上声明 `idx_wp_dict_entries_hash` 唯一索引，与迁移文件一致。
- 重启真实 frontend 与 api-server，触发 AutoMigrate 和默认字典 seed。

## 验证步骤

- `admin/Admin@123` 真实登录成功。
- `http://127.0.0.1:8081/risk/weak-password` 页面可见“智能弱密码检测”。
- `POST /api/v1/weak-password/asset-applications/analyze` 返回真实应用资产候选。
- `GET /api/v1/weak-password/dictionaries/default` 返回 `entry_count: 1000`。
- `PLAYWRIGHT_REAL=1 ... npx playwright test e2e/weak-password-real.spec.ts --reporter=line` 通过。

## 风险与回滚

- 前端文案变更仅影响菜单和页面标题。
- 字典唯一索引只约束同一字典内重复候选 hash，符合去重设计。
- 若需回滚真实容器内二进制，可恢复 `/root/api-server.prev-weakpass-index` 并重启 api-server 容器。
