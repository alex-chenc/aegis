# 规则与任务管理增强设计文档

**版本**: 1.0
**日期**: 2026-03-10
**作者**: Sisyphus

## 1. 概述

本文档定义了工作台规则列表和任务中心的增强功能设计，包括：
- 规则列表的默认脚本编辑功能
- 规则删除功能（带任务关联检查）
- 任务删除和批量删除功能

## 2. 工作台 - 规则列表修改

### 2.1 界面变更

**删除列：**
- "版本" 列 - 移除 `check_script_version` 和 `fix_script_version` 显示

**新增列：**
- "默认脚本" 列 - 包含两个按钮：
  - "检测脚本" 按钮
  - "修复脚本" 按钮

**操作列变更：**
- 移除原有的 "检测" 和 "修复" 查看 按钮
- 移除 "编辑" 按钮
- 新增 "删除" 按钮

### 2.2 脚本编辑功能

**交互流程：**

1. 用户点击 "检测脚本" 或 "修复脚本" 按钮
2. 检查脚本是否已生成：
   - **已生成**: 直接打开编辑对话框
   - **未生成**: 显示加载状态，调用LLM生成脚本，生成完成后打开编辑对话框

**编辑对话框设计：**
```
┌─────────────────────────────────────────────────────────────┐
│  编辑检测脚本 - {规则标题}                          [X]      │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────┐│
│  │ #!/bin/bash                                             ││
│  │ set -e                                                  ││
│  │ # 检查SSH配置                                           ││
│  │ ...                                                     ││
│  │                                                         ││
│  └─────────────────────────────────────────────────────────┘│
│                                                             │
│                              [取消]  [保存]                 │
└─────────────────────────────────────────────────────────────┘
```

**功能特性：**
- 代码编辑器：`<textarea>` 等宽字体，支持滚动
- "保存" 按钮：调用 API 保存脚本内容
- 保存成功后显示成功提示，关闭对话框

### 2.3 规则删除功能

**删除流程：**

1. 用户点击 "删除" 按钮
2. 前端调用 `GET /api/v1/rules/{id}/has-tasks` 检查是否有关联任务
3. 如果有关联任务：
   - 显示错误提示："该规则有关联任务，无法删除"
   - 不执行删除
4. 如果无关联任务：
   - 显示确认对话框："确定删除规则 '{规则标题}'？"
   - 用户确认后调用 `DELETE /api/v1/rules/{id}`
   - 删除成功后刷新规则列表

## 3. 任务中心 - 删除功能

### 3.1 单个删除

**界面变更：**
- 在操作列添加 "删除" 按钮

**删除逻辑：**
- 只允许删除已完成/失败的任务
- 运行中/待执行的任务：按钮置灰禁用，鼠标悬停显示提示 "运行中的任务无法删除"
- 点击删除 → 确认对话框 → 调用 API 删除

### 3.2 批量删除

**界面变更：**
- 添加多选列（checkbox）
- 表格上方添加 "批量删除" 按钮

**批量删除流程：**

1. 用户勾选要删除的任务
2. 点击 "批量删除" 按钮
3. 前端筛选出可删除的任务（仅已完成/失败的）
4. 如果有运行中的任务被选中：
   - 显示通知："已跳过 X 个运行中的任务"
5. 显示确认对话框："确定删除选中的 X 个任务？"
6. 用户确认后调用 `DELETE /api/v1/tasks/batch`
7. 删除成功后刷新列表

## 4. 后端API设计

### 4.1 新增接口

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/rules/{id}/scripts/generate` | 请求LLM生成脚本 |
| PUT | `/api/v1/rules/{id}/scripts` | 更新脚本内容 |
| DELETE | `/api/v1/rules/{id}` | 删除规则（无关联任务时） |
| GET | `/api/v1/rules/{id}/has-tasks` | 检查规则是否有关联任务 |
| DELETE | `/api/v1/tasks/batch` | 批量删除任务 |

### 4.2 接口详细定义

#### POST /api/v1/rules/{id}/scripts/generate

**请求体：**
```json
{
  "script_type": "CHECK" // 或 "FIX"
}
```

**响应体 (200 OK)：**
```json
{
  "code": 0,
  "message": "script generated successfully",
  "data": {
    "rule_id": "uuid",
    "script_type": "CHECK",
    "script_content": "#!/bin/bash\nset -e\n...",
    "version": 1
  }
}
```

#### PUT /api/v1/rules/{id}/scripts

**请求体：**
```json
{
  "script_type": "CHECK",
  "script_content": "#!/bin/bash\nset -e\n..."
}
```

**响应体 (200 OK)：**
```json
{
  "code": 0,
  "message": "script updated successfully"
}
```

#### DELETE /api/v1/rules/{id}

**响应体 (200 OK)：**
```json
{
  "code": 0,
  "message": "rule deleted successfully"
}
```

**响应体 (400 Bad Request - 有关联任务)：**
```json
{
  "code": 400,
  "message": "rule has associated tasks, cannot delete"
}
```

#### GET /api/v1/rules/{id}/has-tasks

**响应体 (200 OK)：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "has_tasks": true,
    "task_count": 5
  }
}
```

#### DELETE /api/v1/tasks/batch

**请求体：**
```json
{
  "task_ids": ["uuid-1", "uuid-2", "uuid-3"]
}
```

**响应体 (200 OK)：**
```json
{
  "code": 0,
  "message": "batch delete completed",
  "data": {
    "deleted_count": 3,
    "skipped_count": 0
  }
}
```

## 5. 数据库变更

无Schema变更，使用现有表结构。

## 6. 前端组件设计

### 6.1 ScriptEditorDialog.vue (新建)

```vue
<template>
  <el-dialog v-model="visible" :title="dialogTitle" width="70%">
    <div v-if="loading" class="loading-container">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>正在请求AI生成脚本...</span>
    </div>
    <div v-else>
      <el-input
        v-model="scriptContent"
        type="textarea"
        :rows="20"
        class="script-editor"
        placeholder="脚本内容"
      />
    </div>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" @click="saveScript" :loading="saving">保存</el-button>
    </template>
  </el-dialog>
</template>
```

### 6.2 Workbench.vue 修改

- 移除版本列
- 添加默认脚本列（合并两个按钮）
- 添加删除操作按钮
- 集成 ScriptEditorDialog 组件

### 6.3 TaskCenter.vue 修改

- 添加多选列
- 添加批量删除按钮
- 操作列添加删除按钮

## 7. 实现计划

详见后续实现计划文档。