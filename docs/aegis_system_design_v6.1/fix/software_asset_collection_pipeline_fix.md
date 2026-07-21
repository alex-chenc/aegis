# 软件资产采集链路修复

## 问题与成功标准

资产采集入口会丢弃 `software` 类型，单主机执行流程也没有调用软件采集工具或写入
`host_software_assets`，因此任务中的 `software_count` 始终为零。前端手动采集、周期配置和
新部署数据库默认值同样未包含该类型。

修复后应满足：

- `software` 和 `full` 请求会保留软件采集语义，同时继续强制采集进程快照；
- 选中软件采集时，通过 Agent 工具 `AssetCollectHostAssets` 获取软件包并 Upsert 到
  `host_software_assets`；采集或持久化失败会使对应主机任务失败；
- 主机任务明细与完成日志记录成功保存的软件数量；
- 手动采集、周期配置及新部署数据库的默认类型统一为
  `["process","software","application_analysis"]`；
- 重试失败主机时复用原任务的采集类型，避免重试再次跳过软件。

## 数据流与实现

```text
Overview/Collections -> api-server TriggerAssetCollection
  -> normalizeCollectTypes
  -> collectHost
     -> AssetCollectProcessSnapshot
     -> AssetCollectHostAssets (software selected)
     -> UpsertSoftwareAsset -> host_software_assets
     -> application analysis (selected)
```

软件采集工具只请求 `software`，API Server 忽略工具返回中的非软件字段。每个软件包沿用
现有 fingerprint 和仓储冲突键进行 Upsert，不改变 API、gRPC 或表结构。

## 测试、兼容性与回滚

- 单元测试覆盖类型规范化、工具名/参数、响应解析和失败响应；
- 运行 `api-server` 相关包测试及构建，并运行前端类型检查和构建；
- 数据库调整新建表的定义，并幂等更新已有表的列默认值，但不覆盖现有用户配置行；旧任务
  仍按自身保存的类型执行，兼容不含 `software` 的历史数据；
- 回滚可移除软件采集分支并恢复默认数组，无需回滚表结构或删除已采集资产。
