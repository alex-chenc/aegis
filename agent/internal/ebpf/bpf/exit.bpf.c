//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define TASK_COMM_LEN 16

struct exit_event {
    __u32 pid;
    __u32 uid;
    __s32 exit_code;
    __u8 comm[TASK_COMM_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} exit_events SEC(".maps");

SEC("tracepoint/sched/sched_process_exit")
int trace_exit(void *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u32 tid = pid_tgid & 0xFFFFFFFF;
    
    // Only capture main thread exit (PID == TID), skip child threads
    if (pid != tid) {
        return 0;
    }
    
    struct exit_event *e;
    
    e = bpf_ringbuf_reserve(&exit_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->pid = pid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->exit_code = 0;
    
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";