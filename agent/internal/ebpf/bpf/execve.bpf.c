//go:build ignore

#include "common.h"
#include "event_output.h"

struct exec_event {
    __u32 pid;
    __u32 ppid;
    __u32 uid;
    __u32 gid;
    __u32 flags;
    __u8 comm[TASK_COMM_LEN];
    __u8 filename[MAX_PATH_LEN];
    __u8 args[MAX_ARGS_LEN];
};

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(exec_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(exec_events);
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct exec_event);
} exec_scratch SEC(".maps");
#endif

static __always_inline __u32 read_argv(__u8 *dst, const char **argv)
{
    __u32 flags = 0;
    const char *arg = 0;

#pragma unroll
    for (int i = 0; i < MAX_ARGC; i++) {
        if (bpf_probe_read_user(&arg, sizeof(arg), &argv[i]) < 0)
            break;
        if (!arg)
            break;

        int read_len = bpf_probe_read_user_str(dst + (i * MAX_SINGLE_ARG_LEN), MAX_SINGLE_ARG_LEN, arg);
        if (read_len <= 0)
            break;
        if (read_len == MAX_SINGLE_ARG_LEN)
            flags |= EXEC_EVENT_ARGS_TRUNCATED;
        if (i == MAX_ARGC - 1) {
            const char *next_arg = 0;
            if (bpf_probe_read_user(&next_arg, sizeof(next_arg), &argv[i + 1]) == 0 && next_arg)
                flags |= EXEC_EVENT_ARGS_TRUNCATED;
        }
    }
    return flags;
}

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_execve(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    const char *filename_ptr;
    const char **argv_ptr;

#if defined(AEGIS_EVENT_RINGBUF)
    struct exec_event *e;
    e = bpf_ringbuf_reserve(&exec_events, sizeof(*e), 0);
    if (!e)
        return 0;
#elif defined(AEGIS_EVENT_PERF)
    __u32 key = 0;
    struct exec_event *e = bpf_map_lookup_elem(&exec_scratch, &key);
    if (!e)
        return 0;
    __builtin_memset(e, 0, sizeof(*e));
#endif

    pid_tgid = bpf_get_current_pid_tgid();
    e->pid = pid_tgid >> 32;
    e->ppid = 0;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->gid = bpf_get_current_uid_gid() >> 32;
    e->flags = 0;
    bpf_get_current_comm(e->comm, sizeof(e->comm));

    filename_ptr = (const char *)ctx->args[0];
    bpf_probe_read_user_str(e->filename, sizeof(e->filename), filename_ptr);

    argv_ptr = (const char **)ctx->args[1];
    if (argv_ptr)
        e->flags |= read_argv(e->args, argv_ptr);

#if defined(AEGIS_EVENT_RINGBUF)
    bpf_ringbuf_submit(e, 0);
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &exec_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
#endif
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
