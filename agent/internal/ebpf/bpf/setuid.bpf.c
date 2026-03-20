//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define TASK_COMM_LEN 16

struct priv_event {
    __u32 pid;
    __u32 uid;
    __u32 target_uid;
    __u8 syscall[16];
    __u8 comm[TASK_COMM_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} priv_events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_setuid")
int trace_setuid(struct trace_event_raw_sys_enter *ctx)
{
    struct priv_event *e;
    
    e = bpf_ringbuf_reserve(&priv_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->pid = bpf_get_current_pid_tgid() >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->target_uid = (__u32)ctx->args[0];
    
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    __builtin_memcpy(e->syscall, "setuid", 7);
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
