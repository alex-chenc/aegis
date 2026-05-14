# AI Analysis Alert Loading Performance Optimization

| Item            | Value                                      |
| --------------- | ------------------------------------------ |
| Document ID     | AEGIS-V5.7-OPT-002                        |
| Version         | v5.7                                       |
| Author          | Claude Code                                |
| Date            | 2026-05-13                                 |
| Status          | Proposed                                   |
| Module          | Frontend - AI Analysis Page                |
| Related Files   | `frontend/src/views/detection/AIAnalysis.vue` |
|                 | `frontend/src/utils/aiAnalysisFilters.ts`  |
|                 | `frontend/src/api/detection.ts`            |

> Note: This document focuses on **frontend reactive layer** optimizations. For backend/database-level optimizations (index creation, subquery elimination), see `ai_analysis_performance_optimization.md`.

---

## 1. Problem Background

### 1.1 Symptom

On the AI Analysis page (`AIAnalysis.vue`), when a user selects a time range or one/more hosts in the filter panel, the alert list loads abnormally slowly. The expected behavior is sub-second response since the backend database query completes in approximately 0.379ms for the 471-alert dataset used in profiling.

### 1.2 Impact

- Users experience noticeable lag (multiple seconds) after each filter change.
- Rapid filter adjustments (e.g., toggling several hosts in quick succession) cause a burst of redundant API calls, further degrading perceived performance.
- The slow feedback loop disrupts the analyst workflow when triaging alerts before initiating AI analysis.

### 1.3 Scope

This optimization targets the **frontend data-loading path** for the alert selection table. It does not cover the AI analysis SSE streaming pipeline, the backend database query layer, or the alert detail view.

---

## 2. Root Cause Analysis

### 2.1 Database Layer -- Not the Bottleneck

The backend `AlertRepository.List()` method executes a `LEFT JOIN` query across `alerts`, `hosts`, and `sigma_rules` tables. Profiling shows:

| Metric          | Value    |
| --------------- | -------- |
| Query time      | ~0.379ms |
| Row count       | 471      |
| Result payload  | ~85 KB   |

The database layer is not a contributing factor.

### 2.2 Frontend Layer -- Three Identified Issues

#### Issue 1: No Debounce on Reactive Watcher

**Location**: `AIAnalysis.vue`, line 1371-1373

```typescript
watch([hostFilter, timeRange], () => {
  loadAlerts()
}, { deep: true })
```

This watcher triggers `loadAlerts()` on **every** change to `hostFilter` (an array) or `timeRange` (a date-range tuple). There is no debounce mechanism. When a user:

1. Selects host "web-server-1" -- triggers API call
2. Immediately selects host "db-server-2" -- triggers another API call
3. Immediately selects host "cache-server-3" -- triggers yet another API call

Each intermediate call is wasted because the final intent is to query all three hosts. The `loadAlertSeq` counter prevents stale responses from overwriting newer ones, but the network requests still fire and consume bandwidth/server resources.

#### Issue 2: Redundant Client-Side Filtering

**Location**: `AIAnalysis.vue`, line 529-531

```typescript
const filteredAlerts = computed(() => {
  return filterAnalysisAlerts(alerts.value, hostFilter.value, timeRange.value)
})
```

The `filterAnalysisAlerts()` function (in `aiAnalysisFilters.ts`, line 58-71) applies hostname and time-range filtering on the already-filtered API response. The backend `getAlerts()` call receives `hostnames` and `start_time`/`end_time` query parameters, meaning the returned data is already filtered. The computed property performs this work again on every render cycle:

```typescript
// aiAnalysisFilters.ts - this runs on every access of filteredAlerts
export function filterAnalysisAlerts<T extends AnalysisAlertLike>(
  alerts: T[],
  hostFilter: string[],
  timeRange?: [string, string] | null
) {
  const hasHostFilter = hostFilter.length > 0
  const hasTimeRange = Boolean(timeRange?.[0] && timeRange?.[1])
  if (!hasHostFilter && !hasTimeRange) return []

  return alerts.filter(alert => {
    const hostMatched = !hasHostFilter || Boolean(alert.hostname && hostFilter.includes(alert.hostname))
    return hostMatched && isInTimeRange(alert, timeRange)
  })
}
```

For 471 alerts with string comparisons and date parsing, this adds measurable overhead on each reactive dependency change.

#### Issue 3: Deep Watch on Arrays

**Location**: `AIAnalysis.vue`, line 1371-1373 and 1358-1360

The `{ deep: true }` option on the watcher causes Vue to recursively observe all nested properties within `hostFilter` (string array) and `timeRange` (date tuple). This means:

- Adding/removing a host triggers the watcher for the array element change **and** the array reference change.
- The deep watch on `messages` (line 1358-1360) for auto-save also fires on every nested property mutation within message objects.

While `hostFilter` and `timeRange` are relatively shallow structures, the `deep: true` flag prevents Vue from short-circuiting when the array reference itself hasn't changed.

### 2.3 Interaction Flow Diagram

```
User clicks host checkbox
        |
        v
hostFilter ref changes (deep=true triggers)
        |
        v
watch([hostFilter, timeRange]) fires immediately
        |
        v
loadAlerts() called -- API request #1
        |
User clicks another host (within 100ms)
        |
        v
hostFilter ref changes again
        |
        v
watch fires again -- API request #2 (request #1 still in flight)
        |
        v
API request #1 returns, but currentSeq !== alertLoadSeq (stale, discarded)
        |
        v
API request #2 returns, updates alerts.value
        |
        v
filteredAlerts computed recomputes (redundant filtering on already-filtered data)
        |
        v
visibleAlertRows computed recomputes
        |
        v
Table re-renders
```

---

## 3. Optimization Design

### 3.1 Overview

| Optimization              | Expected Impact          | Risk Level |
| ------------------------- | ------------------------ | ---------- |
| Add 300ms debounce        | Eliminate N-1 redundant API calls for N rapid changes | Low |
| Remove redundant filter   | Reduce computed property overhead by ~100% | Low |
| Use shallow watch         | Reduce watcher trigger frequency | Low |

### 3.2 Optimization 1: Debounced Watcher

**Goal**: Coalesce rapid filter changes into a single API call.

**Design**: Replace the direct `watch` callback with a debounced function. Use a 300ms debounce window, which is short enough to feel instantaneous but long enough to capture rapid successive selections.

```
Watch callback flow (before):
  change A --> loadAlerts()
  change B --> loadAlerts()   (100ms later)
  change C --> loadAlerts()   (200ms later)
  Result: 3 API calls

Watch callback flow (after, 300ms debounce):
  change A --> schedule debounce timer
  change B --> reset debounce timer
  change C --> reset debounce timer
  (300ms after change C)
  --> loadAlerts()
  Result: 1 API call
```

### 3.3 Optimization 2: Remove Redundant Client-Side Filtering

**Goal**: Eliminate the double-filtering pattern where the frontend filters data that the backend has already filtered.

**Design**: Since `buildAnalysisAlertQuery()` already passes `hostnames`, `start_time`, and `end_time` to the backend, the API response (`alerts.value`) is already filtered. The `filteredAlerts` computed property should return `alerts.value` directly when a query condition exists, bypassing `filterAnalysisAlerts()`.

The `filterAnalysisAlerts()` utility function is retained for edge cases (e.g., when `loadAlerts()` is called with `force=true` and no query parameters, returning unfiltered data that still needs client-side filtering), but the primary data path no longer invokes it.

### 3.4 Optimization 3: Shallow Watch

**Goal**: Reduce unnecessary watcher triggers from deep observation of array/date internals.

**Design**: Change `{ deep: true }` to `{ deep: false }` (the default) for the `[hostFilter, timeRange]` watcher. Since both are replaced by reference when modified (Vue reactivity on `ref` values), the shallow watch captures all meaningful changes. The `messages` watcher for auto-save retains `deep: true` because message objects are mutated in-place.

---

## 4. Implementation Details

### 4.1 File: `frontend/src/views/detection/AIAnalysis.vue`

#### Change 1: Add debounce utility import

Add at the top of the `<script setup>` block:

```typescript
import { useDebounceFn } from '@vueuse/core'
// OR, if @vueuse/core is not available, implement inline:
function debounce<T extends (...args: any[]) => void>(fn: T, ms: number): T {
  let timer: ReturnType<typeof setTimeout> | null = null
  return ((...args: any[]) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => fn(...args), ms)
  }) as unknown as T
}
```

Note: Check `package.json` before choosing an implementation. If `@vueuse/core` is not available, the inline implementation avoids adding a new dependency.

#### Change 2: Replace the watcher (line 1371-1373)

**Before:**
```typescript
watch([hostFilter, timeRange], () => {
  loadAlerts()
}, { deep: true })
```

**After:**
```typescript
const debouncedLoadAlerts = useDebounceFn(() => {
  loadAlerts()
}, 300)

watch([hostFilter, timeRange], () => {
  debouncedLoadAlerts()
})
```

#### Change 3: Simplify `filteredAlerts` computed (line 529-531)

**Before:**
```typescript
const filteredAlerts = computed(() => {
  return filterAnalysisAlerts(alerts.value, hostFilter.value, timeRange.value)
})
```

**After:**
```typescript
const filteredAlerts = computed(() => {
  // API already filters by hostnames and time_range via buildAnalysisAlertQuery.
  // Only apply client-side filtering when no query was sent (force-load edge case).
  const query = buildAnalysisAlertQuery(hostFilter.value, timeRange.value)
  if (query) {
    return alerts.value
  }
  return filterAnalysisAlerts(alerts.value, hostFilter.value, timeRange.value)
})
```

#### Change 4: Optional -- debounce the messages auto-save watcher (line 1358-1360)

```typescript
const debouncedSaveConversation = useDebounceFn(() => {
  saveConversation()
}, 500)

watch(messages, () => {
  debouncedSaveConversation()
}, { deep: true })
```

### 4.2 File: `frontend/src/utils/aiAnalysisFilters.ts`

No changes required. The `filterAnalysisAlerts()` function is retained as-is for the fallback path.

### 4.3 File: `frontend/src/api/detection.ts`

No changes required. The `getAlerts()` API function already supports the query parameters needed.

---

## 5. Test Plan

### 5.1 Unit Tests

#### Test Case 1: Debounce Behavior

```
Scenario: Rapid host selection
  Given: User opens AI Analysis page with alerts loaded
  When: User selects host "web-1", then "web-2" within 100ms
  Then: Only 1 API call to getAlerts() is made after 300ms debounce
  And: The final call includes both hosts in the hostnames parameter
```

#### Test Case 2: Debounce Cancellation

```
Scenario: Filter change followed by immediate reset
  Given: User opens AI Analysis page
  When: User selects a host, then clears the filter within 300ms
  Then: No API call is made (debounce cancels the first trigger)
```

#### Test Case 3: No Redundant Filtering

```
Scenario: filteredAlerts returns API data directly
  Given: API returns 100 alerts for host "web-1"
  When: filteredAlerts computed property is evaluated
  Then: It returns the same 100 alerts without re-filtering
  And: filterAnalysisAlerts() is NOT called
```

#### Test Case 4: Fallback Filtering

```
Scenario: Force load without query parameters
  Given: loadAlerts(true) is called with no host/time filters
  When: filteredAlerts computed property is evaluated
  Then: filterAnalysisAlerts() IS called for client-side filtering
```

### 5.2 Integration Tests

#### Test Case 5: End-to-End Alert Loading

```
Scenario: Full filter workflow
  Given: User is on AI Analysis page
  When: User selects time range "2026-05-01 to 2026-05-13"
  And: User selects hosts "web-1" and "db-1"
  Then: Alert table shows only alerts matching both filters
  And: Total API calls made = 2 (one for time range, one for hosts+time range)
  And: Each API call completes within 1 second
```

#### Test Case 6: Selection Pruning After Filter Change

```
Scenario: Selected alerts survive filter change
  Given: User has selected alert IDs [1, 2, 3]
  When: User changes host filter to exclude alert #3
  Then: selectedAlertIds is pruned to [1, 2]
  And: Table selection reflects the pruned list
```

### 5.3 Performance Benchmarks

| Metric                          | Before (Expected) | After (Target)  |
| ------------------------------- | ------------------ | --------------- |
| API calls for 3 rapid selections | 3                  | 1               |
| Time to stable UI after filter   | ~1.5s              | <500ms          |
| `filteredAlerts` compute time    | ~2-5ms per call    | ~0ms (passthrough) |
| Watcher trigger count per change | 2-3 (deep)         | 1 (shallow)     |

### 5.4 Regression Checklist

- [ ] Manual refresh button still works (calls `loadAlerts()` directly, bypasses debounce)
- [ ] Page load with `alert_ids` query parameter still auto-selects and starts analysis
- [ ] Session restore from localStorage still works
- [ ] Analysis snapshot mode (`isAnalysisSnapshotActive`) still displays snapshot alerts
- [ ] `pruneSelectionToVisibleAlerts()` still triggers on `filteredAlerts` change
- [ ] No duplicate alerts appear in the table
- [ ] Empty state (no filters selected) correctly shows no alerts

---

## 6. Risk Assessment

| Risk                                    | Likelihood | Mitigation                                   |
| --------------------------------------- | ---------- | -------------------------------------------- |
| Debounce delays legitimate single-click | Low        | 300ms is below perception threshold; refresh button provides immediate load |
| Removing filter breaks edge case        | Low        | Fallback path retained for force-load scenario |
| Shallow watch misses nested mutation    | Low        | `hostFilter` and `timeRange` are replaced by reference, not mutated in-place |

---

## 7. Migration Notes

- No database migration required.
- No API contract changes required.
- No new dependencies required (inline debounce implementation used if `@vueuse/core` is unavailable).
- Changes are backward-compatible with existing frontend build pipeline.
