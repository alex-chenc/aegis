//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define TASK_COMM_LEN 16

struct cap_event {
    __u32 pid;
    __u32 uid;
    __u64 cap_effective;
    __u64 cap_permitted;
    __u8 syscall[16];
    __u8 comm[TASK_COMM_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} cap_events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_capset")
int trace_capset(struct trace_event_raw_sys_enter *ctx)
{
    struct cap_event *e;
    
    e = bpf_ringbuf_reserve(&cap_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->pid = bpf_get_current_pid_tgid() >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    
    /* args: hdrp, effective, permitted, inheritable */
    bpf_probe_read_user(&e->cap_effective, sizeof(e->cap_effective),
                        (void *)ctx->args[1]);
    bpf_probe_read_user(&e->cap_permitted, sizeof(e->cap_permitted),
                        (void *)ctx->args[2]);
    
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    __builtin_memcpy(e->syscall, "capset", 7);
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
