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

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} fork_events SEC(".maps");

SEC("tracepoint/sched/sched_process_fork")
int trace_fork(void *ctx)
{
    struct fork_event *e;
    struct task_struct *task;
    __u64 pid_tgid;
    __u32 pid, child_pid;
    
    e = bpf_ringbuf_reserve(&fork_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    pid_tgid = bpf_get_current_pid_tgid();
    pid = pid_tgid >> 32;
    child_pid = (__u32)pid_tgid;
    
    e->parent_pid = pid;
    e->child_pid = child_pid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    
    bpf_get_current_comm(e->parent_comm, sizeof(e->parent_comm));
    
    task = (struct task_struct *)bpf_get_current_task();
    bpf_probe_read_kernel_str(e->child_comm, sizeof(e->child_comm),
                              BPF_CORE_READ(task, comm));
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
