# Code Review: NotificationDrawer Layout Fix

**Reviewed**: 2026-05-06
**Branch**: develop
**Decision**: APPROVE

## Summary

修复通知抽屉显示位置问题。通过添加 `:append-to-body="true"` 属性，使 Element Plus 的 el-drawer 组件渲染到 document.body，脱离父容器的层叠上下文，解决抽屉内容显示在页面底部不可见的问题。

## Findings

### CRITICAL
None

### HIGH
None

### MEDIUM
None

### LOW
None

## Validation Results

| Check | Result |
|-------|--------|
| Tests | Pass (4/4) |
| Build | Pass |

## Files Reviewed

| File | Change Type |
|------|-------------|
| `frontend/src/components/notification/NotificationDrawer.vue` | Modified |
| `frontend/src/components/notification/NotificationDrawer.test.ts` | Added |
| `docs/aegis_system_design_v5.6/notification_drawer_layout_fix_design.md` | Added |

## Change Details

```diff
  <el-drawer
    v-model="store.drawerVisible"
    title="消息通知"
    direction="rtl"
    size="480px"
+   :append-to-body="true"
    @close="handleClose"
  >
```
