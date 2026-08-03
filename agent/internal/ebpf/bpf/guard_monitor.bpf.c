//go:build ignore

#include "common.h"
#include "event_output.h"

#define GUARD_OP_SETUID            1
#define GUARD_OP_SETGID            2
#define GUARD_OP_CAPSET            3
#define GUARD_OP_SETNS             4
#define GUARD_OP_UNSHARE           5
#define GUARD_OP_CLONE3            6
#define GUARD_OP_MOUNT             7
#define GUARD_OP_PIVOT_ROOT        8
#define GUARD_OP_CHROOT            9
#define GUARD_OP_PTRACE           10
#define GUARD_OP_BPF              11
#define GUARD_OP_PERF_EVENT_OPEN  12
#define GUARD_OP_INIT_MODULE      13
#define GUARD_OP_FINIT_MODULE     14
#define GUARD_OP_DELETE_MODULE    15
#define GUARD_OP_CONNECT_UNIX     16

#define GUARD_FLAG_TARGET_TRUNCATED    (1U << 0)
#define GUARD_FLAG_SECONDARY_TRUNCATED (1U << 1)

struct guard_monitor_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __u32 operation;
    __u32 flags;
    __u64 arg0;
    __u64 arg1;
    __u64 arg2;
    __s64 return_code;
    __u8 comm[TASK_COMM_LEN];
    __u8 target[MAX_PATH_LEN];
    __u8 secondary[MAX_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 8192);
    __type(key, __u64);
    __type(value, struct guard_monitor_event);
} guard_pending SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct guard_monitor_event);
} guard_entry_scratch SEC(".maps");

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(guard_monitor_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(guard_monitor_events);
#endif

static __always_inline int guard_record_enter(
    struct trace_event_raw_sys_enter *ctx,
    __u32 operation,
    const char *target,
    const char *secondary)
{
    __u32 key = 0;
    struct guard_monitor_event *event =
        bpf_map_lookup_elem(&guard_entry_scratch, &key);
    if (!event)
        return 0;

    __builtin_memset(event, 0, sizeof(*event));
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 uid_gid = bpf_get_current_uid_gid();
    event->timestamp_ns = bpf_ktime_get_ns();
    event->pid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->uid = (__u32)uid_gid;
    event->gid = uid_gid >> 32;
    event->operation = operation;
    event->arg0 = ctx->args[0];
    event->arg1 = ctx->args[1];
    event->arg2 = ctx->args[2];
    bpf_get_current_comm(event->comm, sizeof(event->comm));

    if (target) {
        int length = bpf_probe_read_user_str(event->target, sizeof(event->target), target);
        if (length == sizeof(event->target))
            event->flags |= GUARD_FLAG_TARGET_TRUNCATED;
    }
    if (secondary) {
        int length = bpf_probe_read_user_str(event->secondary, sizeof(event->secondary), secondary);
        if (length == sizeof(event->secondary))
            event->flags |= GUARD_FLAG_SECONDARY_TRUNCATED;
    }

    bpf_map_update_elem(&guard_pending, &pid_tgid, event, BPF_ANY);
    return 0;
}

static __always_inline int guard_record_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct guard_monitor_event *pending =
        bpf_map_lookup_elem(&guard_pending, &pid_tgid);
    if (!pending)
        return 0;
    pending->return_code = ctx->ret;

#if defined(AEGIS_EVENT_RINGBUF)
    struct guard_monitor_event *output =
        bpf_ringbuf_reserve(&guard_monitor_events, sizeof(*output), 0);
    if (output) {
        __builtin_memcpy(output, pending, sizeof(*output));
        bpf_ringbuf_submit(output, 0);
    }
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &guard_monitor_events, BPF_F_CURRENT_CPU,
                          pending, sizeof(*pending));
#endif
    bpf_map_delete_elem(&guard_pending, &pid_tgid);
    return 0;
}

#define DEFINE_GUARD_SYSCALL(name, operation, target, secondary) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int guard_enter_##name(struct trace_event_raw_sys_enter *ctx) \
{ \
    return guard_record_enter(ctx, operation, target, secondary); \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int guard_exit_##name(struct trace_event_raw_sys_exit *ctx) \
{ \
    return guard_record_exit(ctx); \
}

DEFINE_GUARD_SYSCALL(setuid, GUARD_OP_SETUID, NULL, NULL)
DEFINE_GUARD_SYSCALL(setgid, GUARD_OP_SETGID, NULL, NULL)
DEFINE_GUARD_SYSCALL(capset, GUARD_OP_CAPSET, NULL, NULL)
DEFINE_GUARD_SYSCALL(setns, GUARD_OP_SETNS, NULL, NULL)
DEFINE_GUARD_SYSCALL(unshare, GUARD_OP_UNSHARE, NULL, NULL)
DEFINE_GUARD_SYSCALL(clone3, GUARD_OP_CLONE3, NULL, NULL)
DEFINE_GUARD_SYSCALL(mount, GUARD_OP_MOUNT,
                     (const char *)ctx->args[1],
                     (const char *)ctx->args[0])
DEFINE_GUARD_SYSCALL(pivot_root, GUARD_OP_PIVOT_ROOT,
                     (const char *)ctx->args[0],
                     (const char *)ctx->args[1])
DEFINE_GUARD_SYSCALL(chroot, GUARD_OP_CHROOT,
                     (const char *)ctx->args[0], NULL)
DEFINE_GUARD_SYSCALL(ptrace, GUARD_OP_PTRACE, NULL, NULL)
DEFINE_GUARD_SYSCALL(bpf, GUARD_OP_BPF, NULL, NULL)
DEFINE_GUARD_SYSCALL(perf_event_open, GUARD_OP_PERF_EVENT_OPEN, NULL, NULL)
DEFINE_GUARD_SYSCALL(init_module, GUARD_OP_INIT_MODULE, NULL, NULL)
DEFINE_GUARD_SYSCALL(finit_module, GUARD_OP_FINIT_MODULE, NULL, NULL)
DEFINE_GUARD_SYSCALL(delete_module, GUARD_OP_DELETE_MODULE,
                     (const char *)ctx->args[0], NULL)

SEC("tracepoint/syscalls/sys_enter_connect")
int guard_enter_connect(struct trace_event_raw_sys_enter *ctx)
{
    guard_record_enter(ctx, GUARD_OP_CONNECT_UNIX, NULL, NULL);
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct guard_monitor_event *event =
        bpf_map_lookup_elem(&guard_pending, &pid_tgid);
    if (!event)
        return 0;
    __u16 family = 0;
    const void *address = (const void *)ctx->args[1];
    if (!address || bpf_probe_read_user(&family, sizeof(family), address) < 0 || family != 1) {
        bpf_map_delete_elem(&guard_pending, &pid_tgid);
        return 0;
    }
    int length = bpf_probe_read_user_str(event->target, sizeof(event->target),
                                         (const char *)address + sizeof(family));
    if (length == sizeof(event->target))
        event->flags |= GUARD_FLAG_TARGET_TRUNCATED;
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_connect")
int guard_exit_connect(struct trace_event_raw_sys_exit *ctx)
{
    return guard_record_exit(ctx);
}

char LICENSE[] SEC("license") = "GPL";
