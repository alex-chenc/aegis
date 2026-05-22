//go:build ignore

#include "common.h"
#include "event_output.h"

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
    char parent_comm[TASK_COMM_LEN];
    __u32 parent_pid;
    char child_comm[TASK_COMM_LEN];
    __u32 child_pid;
};

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(fork_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(fork_events);
#endif

SEC("tracepoint/sched/sched_process_fork")
int trace_fork(struct sched_process_fork_args *ctx)
{
    struct fork_event e = {};

    e.parent_pid = ctx->parent_pid;
    e.child_pid = ctx->child_pid;
    e.uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;

    bpf_probe_read_kernel_str(e.parent_comm, sizeof(e.parent_comm), ctx->parent_comm);
    bpf_probe_read_kernel_str(e.child_comm, sizeof(e.child_comm), ctx->child_comm);

#if defined(AEGIS_EVENT_RINGBUF)
    struct fork_event *ring_e;
    ring_e = bpf_ringbuf_reserve(&fork_events, sizeof(*ring_e), 0);
    if (!ring_e)
        return 0;
    __builtin_memcpy(ring_e, &e, sizeof(e));
    bpf_ringbuf_submit(ring_e, 0);
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &fork_events, BPF_F_CURRENT_CPU, &e, sizeof(e));
#endif
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
