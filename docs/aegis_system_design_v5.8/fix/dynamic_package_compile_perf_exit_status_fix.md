# 动态检测包 compile perf: exit status 1 排查与修复方案

## 问题对象

- Package ID: `b1c4300a-d050-4b12-8b0f-b41fce167b1e`
- Build ID: `f90a2e22-ea28-49ce-9758-6817f315cd9a`
- Draft ID: `36edd0ac-314c-4f0b-8f71-4f3e36706669`
- 版本: `1.0.0`
- 前端错误: `compile perf: exit status 1`

## 排查结论

这个错误不是 builder 工作目录残留产物导致的同名冲突，而是该动态检测包草稿中的 eBPF 源码本身无法在 builder 中完成 perf transport 编译。

复现编译后，真实 clang 错误为：

```text
error: call to undeclared function 'bpf_get_current_task'
error: incomplete definition of type 'struct task_struct'
```

同时数据库中的 `detection_package_builds.build_log` 为空，说明 builder 在编译失败时只向 API Server 返回了 `compile perf: exit status 1`，没有把 clang stderr 通过 `BuildLogTail` 或 `build_log_object_key` 落库。因此页面只能展示泛化错误，无法展示真实编译原因。

## 新增 validation 错误排查

Claude 修复后，builder 已经把 `bpf_get_current_task` 加入 `validateBuildInput` 的禁用 helper 列表。因此同一个草稿再次构建时，错误从 clang 阶段的：

```text
compile perf: exit status 1
```

变成了 validation 阶段的：

```text
validation: forbidden BPF helper call: bpf_get_current_task
```

这是预期的拦截结果，不是新的 eBPF 编译问题。新的构建记录为：

```text
id:            9095a13e-d0c9-4c9d-9f18-436212465628
package_id:    b1c4300a-d050-4b12-8b0f-b41fce167b1e
status:        failed
error_message: validation: forbidden BPF helper call: bpf_get_current_task
build_log:     empty
```

数据库中最新草稿 `36edd0ac-314c-4f0b-8f71-4f3e36706669` 仍然包含旧代码：

```c
struct task_struct *task = (struct task_struct *)bpf_get_current_task();
*pid = task->tgid;
*tid = task->pid;
```

并且仍未包含 `BPF_MAP_TYPE_PERF_EVENT_ARRAY`、`AEGIS_EVENT_PERF`、`AEGIS_EVENT_RINGBUF`。因此当前需要修复的是草稿源码本身，而不是移除 builder 的 validation 规则。

Claude 修复有效的部分：

- `bpf_get_current_task` 被提前拦截，错误信息比 `compile perf: exit status 1` 更明确。
- perf/ringbuf 编译失败路径已经补充 `BuildLogTail` 和 MinIO build log 上传。
- clang 参数已经切换为 `AEGIS_EVENT_PERF` / `AEGIS_EVENT_RINGBUF`，并使用共享 eBPF include 目录。

本次继续补齐的部分：

- validation 失败也应该填充 `BuildLogTail`，否则页面仍然显示空日志。
- `populateFailedBuildLog` 应处理 MinIO client 不可用的场景，避免诊断增强路径影响本地测试。
- AI 生成模板应明确禁止 `bpf_get_current_task` 和 `task_struct` 直接解引用，并要求 perf/ringbuf 双 transport 条件编译。

代码处理：

- `builder/internal/service/builder_service.go` 已在 validation 失败路径写入诊断日志，并设置 `BuildLogTail`。
- `populateFailedBuildLog` 已增加 MinIO client nil guard；本地单测不会因为没有 MinIO client 崩溃。
- `api-server/internal/llm/prompts.go` 已更新动态检测包生成约束，要求 perf/ringbuf 双 transport，并禁止 `bpf_get_current_task`。

## 现场证据

### 1. 构建记录

`detection_package_builds` 中最近失败记录：

```text
id:            f90a2e22-ea28-49ce-9758-6817f315cd9a
package_id:    b1c4300a-d050-4b12-8b0f-b41fce167b1e
version:       1.0.0
status:        failed
error_message: compile perf: exit status 1
build_log:     empty
```

### 2. 草稿源码问题

草稿源码中有如下逻辑：

```c
static __always_inline void get_task_info(u32 *pid, u32 *tid) {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    *pid = task->tgid;
    *tid = task->pid;
}
```

问题：

- `bpf_get_current_task` 不是当前 builder 最小 eBPF 头文件集中声明的 helper。
- 源码没有包含完整 `vmlinux.h`/BTF 类型定义，`struct task_struct` 在 clang 视角是不完整类型。
- 直接解引用 `task_struct` 会把草稿和特定内核结构绑定，不符合 V5.8 动态包尽量使用稳定 tracepoint 参数与通用 helper 的原则。

### 3. transport 不一致风险

该源码只定义了 ringbuf map：

```c
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 20);
} events SEC(".maps");
```

但 builder 会分别编译：

```text
-DAEGIS_EVENT_PERF=1
-DAEGIS_EVENT_RINGBUF=1
```

如果源码忽略 `AEGIS_EVENT_PERF`，即使后续修掉 `task_struct` 问题，perf artifact 也可能仍然包含 ringbuf map，导致“文件名是 perf，实际 transport 是 ringbuf”的隐患。

### 4. 运行中的 builder 镜像不是新基础镜像

当前运行中的 `aegis-builder` 容器仍是旧 Alpine 构建环境：

```text
Alpine clang version 21.1.2
AEGIS_EBPF_INCLUDE=unset
```

这和新构建的 `aegis-agent-builder-ubi8:5.8.0` UBI8 基础镜像不一致。即使镜像已构建完成，也需要重启/重建运行中的 builder 服务，实际构建请求才会使用新工具链和共享头文件。

## 修复方案

### Fix 1: 修复该包的 eBPF 源码

不要通过 `bpf_get_current_task()` 读取 `task_struct`。对于 pid/tid，直接使用稳定 helper：

```c
static __always_inline void get_task_info(u32 *pid, u32 *tid) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    *pid = pid_tgid >> 32;
    *tid = (u32)pid_tgid;
}
```

同时将事件 map 和提交逻辑改为 perf/ringbuf 条件编译：

```c
#if defined(AEGIS_EVENT_RINGBUF)
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 20);
} events SEC(".maps");
#elif defined(AEGIS_EVENT_PERF)
struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} events SEC(".maps");
#endif
```

每个 tracepoint 中使用栈上事件作为 perf 输出缓冲，ringbuf 时再 reserve：

```c
struct event stack_event = {};
struct event *e = &stack_event;

#if defined(AEGIS_EVENT_RINGBUF)
e = bpf_ringbuf_reserve(&events, sizeof(struct event), 0);
if (!e)
    return 0;
#endif

/* fill event fields */

#if defined(AEGIS_EVENT_RINGBUF)
bpf_ringbuf_submit(e, 0);
#elif defined(AEGIS_EVENT_PERF)
bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, e, sizeof(*e));
#endif
```

本次排查已用上述方式生成修复版源码，并在 `aegis-agent-builder-ubi8:5.8.0` 中验证：

```text
perf:    ELF 64-bit LSB relocatable, eBPF, with debug_info
ringbuf: ELF 64-bit LSB relocatable, eBPF, with debug_info
```

### Fix 2: 重启 builder 到新 UBI8 构建镜像

镜像构建完成后，需要让运行中的 builder 服务切到新镜像：

```bash
docker compose up -d --no-deps --build builder
```

启动后确认：

```bash
docker exec aegis-builder sh -lc 'go version && clang --version | head -1 && echo AEGIS_EBPF_INCLUDE=$AEGIS_EBPF_INCLUDE'
```

期望：

```text
go version go1.25.0 linux/amd64
clang version 21.1.8
AEGIS_EBPF_INCLUDE=/opt/aegis/ebpf/include
```

### Fix 3: builder 编译失败时返回真实 clang 日志

当前 `builder/internal/service/builder_service.go` 在 perf/ringbuf 编译失败时只设置：

```go
result.ErrorMessage = fmt.Sprintf("compile perf: %v", err)
```

应同时设置：

```go
result.BuildLogTail = tailBuildLog(buildLog.String(), 4096)
result.ClangVersion = parseClangVersion("")
result.BuilderImageDigest = os.Getenv("BUILDER_IMAGE_DIGEST")
```

并建议把失败日志也写入 MinIO：

```go
logObjectKey := fmt.Sprintf("detection-packages/%s/%s/build.log", req.PackageID, req.Version)
logFile := filepath.Join(buildDir, "build.log")
_ = os.WriteFile(logFile, []byte(buildLog.String()), 0644)
_ = s.minioClient.UploadFile(ctx, "aegis-builds", logObjectKey, logFile)
result.BuildLogObjectKey = logObjectKey
```

这样 API Server 现有逻辑可以把 `BuildLogTail` 保存到 `detection_package_builds.build_log`，前端详情页就能看到真实 clang stderr。

### Fix 4: 调整 AI 生成模板和构建前校验

AI 生成动态 eBPF 草稿时应遵守：

- 优先使用 `bpf_get_current_pid_tgid()`、tracepoint 参数和稳定 helper。
- 不直接解引用 `struct task_struct`、`struct sock` 等内核内部结构，除非明确提供 CO-RE/vmlinux 类型和兼容性策略。
- 必须支持 `AEGIS_EVENT_PERF` 与 `AEGIS_EVENT_RINGBUF` 双 transport。
- 输出 map 名称必须与 `plugin.yaml` 的 `event_map` 一致。

builder 的 `validateBuildInput` 建议增加：

```text
1. 禁止或警告 bpf_get_current_task + task_struct 直接解引用。
2. 如果源码包含 bpf_ringbuf_*，必须同时包含 AEGIS_EVENT_RINGBUF 条件分支。
3. 如果源码需要 perf artifact，必须包含 BPF_MAP_TYPE_PERF_EVENT_ARRAY 或 EVENT_OUTPUT 抽象。
4. 编译前记录完整 clang 命令，编译失败时返回 stderr。
```

## 对该 Package 的处理步骤

1. 打开 package `b1c4300a-d050-4b12-8b0f-b41fce167b1e` 的草稿编辑页。
2. 将 `get_task_info()` 替换为 `bpf_get_current_pid_tgid()` 版本。
3. 将 `events` map 和事件提交逻辑改为 perf/ringbuf 条件编译。
4. 确认 builder 服务已经重启到 UBI8 基础镜像。
5. 重新提交构建。
6. 构建失败时检查 `build_log` 或 MinIO `aegis-builds/.../build.log`，不再只看 `compile perf: exit status 1`。

## 验证命令

使用导出的草稿源码复现：

```bash
clang -O2 -g -target bpf \
  -mllvm -bpf-stack-size=1024 \
  -D__TARGET_ARCH_x86 \
  -DAEGIS_EVENT_PERF=1 \
  -I/usr/include \
  -I/usr/include/x86_64-linux-gnu \
  -I/opt/aegis/ebpf/include \
  -c plugin.c \
  -o plugin.perf.bpf.o
```

修复前输出：

```text
error: call to undeclared function 'bpf_get_current_task'
error: incomplete definition of type 'struct task_struct'
```

修复后 perf/ringbuf 均生成 eBPF ELF object。

## 风险

- 修改草稿源码后，检测语义保持一致，只改变 pid/tid 获取方式和 transport 输出方式，风险较低。
- builder 日志增强只影响失败诊断，不改变成功包内容。
- 重启 builder 会中断正在进行的构建任务，建议确认没有 running build 后执行。
