//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#define TASK_COMM_LEN 16

struct fork_event {
    __u32 parent_pid;
    __u32 child_pid;
    __u32 uid;
    __u8 parent_comm[TASK_COMM_LEN];
    __u8 child_comm[TASK_COMM_LEN];
};

// Tracepoint argument structure for sched_process_fork
struct sched_process_fork_args {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;
    unsigned char common_preempt_lazy_count;
    char parent_comm[TASK_COMM_LEN];
    __u32 parent_pid;
    char child_comm[TASK_COMM_LEN];
    __u32 child_pid;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} fork_events SEC(".maps");

SEC("tracepoint/sched/sched_process_fork")
int trace_fork(struct sched_process_fork_args *ctx)
{
    struct fork_event *e;
    
    e = bpf_ringbuf_reserve(&fork_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->parent_pid = ctx->parent_pid;
    e->child_pid = ctx->child_pid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    
    bpf_probe_read_kernel_str(e->parent_comm, sizeof(e->parent_comm), ctx->parent_comm);
    bpf_probe_read_kernel_str(e->child_comm, sizeof(e->child_comm), ctx->child_comm);
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";