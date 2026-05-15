//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define TASK_COMM_LEN 16
#define MAX_PATH_LEN 256
#define MAX_ARGS_LEN 512
#define MAX_ARGC 20
#define EXEC_EVENT_ARGS_TRUNCATED (1U << 0)

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

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} exec_events SEC(".maps");

static __always_inline __u32 read_argv(__u8 *dst, const char **argv)
{
    __u32 offset = 0;
    __u32 flags = 0;
    const char *arg = 0;

#pragma unroll
    for (int i = 0; i < MAX_ARGC; i++) {
        if (offset >= MAX_ARGS_LEN - 1) {
            flags |= EXEC_EVENT_ARGS_TRUNCATED;
            break;
        }

        if (bpf_probe_read_user(&arg, sizeof(arg), &argv[i]) < 0)
            break;
        if (!arg)
            break;

        __u32 remaining = MAX_ARGS_LEN - offset;
        int read_len = bpf_probe_read_user_str(dst + offset, remaining, arg);
        if (read_len <= 0)
            break;
        if (read_len > remaining)
            break;
        if (read_len == remaining) {
            flags |= EXEC_EVENT_ARGS_TRUNCATED;
            break;
        }

        offset += (__u32)read_len;

        if (i == MAX_ARGC - 1) {
            const char *next_arg = 0;
            if (bpf_probe_read_user(&next_arg, sizeof(next_arg), &argv[i + 1]) == 0 && next_arg)
                flags |= EXEC_EVENT_ARGS_TRUNCATED;
        }
    }

    if (offset >= MAX_ARGS_LEN)
        dst[MAX_ARGS_LEN - 1] = 0;

    return flags;
}

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_execve(struct trace_event_raw_sys_enter *ctx)
{
    struct exec_event *e;
    __u64 pid_tgid;
    const char *filename_ptr;
    const char **argv_ptr;
    
    e = bpf_ringbuf_reserve(&exec_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    pid_tgid = bpf_get_current_pid_tgid();
    e->pid = pid_tgid >> 32;
    e->ppid = 0;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->gid = bpf_get_current_uid_gid() >> 32;
    e->flags = 0;
    
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    
    __builtin_memset(e->filename, 0, sizeof(e->filename));
    __builtin_memset(e->args, 0, sizeof(e->args));
    
    filename_ptr = (const char *)ctx->args[0];
    bpf_probe_read_user_str(&e->filename, sizeof(e->filename), filename_ptr);
    
    argv_ptr = (const char **)ctx->args[1];
    if (argv_ptr) {
        e->flags |= read_argv(e->args, argv_ptr);
    }
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
