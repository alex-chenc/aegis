//go:build ignore

#include "common.h"
#include "event_output.h"

// The supported UBI 8 builder ships an older libbpf without bpf_core_read.h.
// Keep the two required CO-RE operations local while still emitting BTF field
// relocations through clang's preserve_access_index builtin.
#define aegis_core_read(dst, size, src) \
    bpf_probe_read_kernel((dst), (size), __builtin_preserve_access_index(src))
#define aegis_core_read_str(dst, size, src) \
    bpf_probe_read_kernel_str((dst), (size), __builtin_preserve_access_index(src))

struct fork_event {
    __u32 event_type;
    __u32 pid;
    __u32 ppid;
    __u32 uid;
    __u8 comm[TASK_COMM_LEN];
    __u8 parent_comm[TASK_COMM_LEN];
};

#define FORK_EVENT_TYPE_FORK 1
#define FORK_EVENT_TYPE_EXIT 2

struct guard_subject {
    __u64 instance_slot;
    __u64 unit_slot;
    __u64 policy_slot;
    __u64 process_epoch;
    __u32 flags;
    __u32 pad;
};

// These maps are reused by the LSM collection. Keeping fork propagation in
// kernel closes the user-space reconciliation window for newly created
// descendants.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u32);
    __type(value, struct guard_subject);
} guarded_pids SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16384);
    __type(key, __u64);
    __type(value, struct guard_subject);
} guarded_cgroups SEC(".maps");

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(fork_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(fork_events);
#endif

SEC("raw_tracepoint/sched_process_fork")
int trace_fork(struct bpf_raw_tracepoint_args *ctx)
{
    struct fork_event e = {};
    struct task_struct *parent = (struct task_struct *)ctx->args[0];
    struct task_struct *child = (struct task_struct *)ctx->args[1];

    if (!parent || !child)
        return 0;

    e.event_type = FORK_EVENT_TYPE_FORK;
    if (aegis_core_read(&e.pid, sizeof(e.pid), &child->tgid) != 0 ||
        aegis_core_read(&e.ppid, sizeof(e.ppid), &parent->tgid) != 0)
        return 0;
    e.uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;

    struct guard_subject *parent_subject =
        bpf_map_lookup_elem(&guarded_pids, &e.ppid);
    if (parent_subject)
        bpf_map_update_elem(&guarded_pids, &e.pid, parent_subject, BPF_ANY);

    aegis_core_read_str(e.parent_comm, sizeof(e.parent_comm), &parent->comm);
    aegis_core_read_str(e.comm, sizeof(e.comm), &child->comm);

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

SEC("raw_tracepoint/sched_process_exit")
int trace_guarded_process_exit(struct bpf_raw_tracepoint_args *ctx)
{
    struct task_struct *task = (struct task_struct *)ctx->args[0];
    struct task_struct *parent = 0;
    __u32 tgid = 0;
    __u32 tid = bpf_get_current_pid_tgid();

    if (!task || aegis_core_read(&tgid, sizeof(tgid), &task->tgid) != 0)
        return 0;
    if (tgid != tid)
        return 0;

    struct fork_event e = {};
    e.event_type = FORK_EVENT_TYPE_EXIT;
    e.pid = tgid;
    e.uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    aegis_core_read_str(e.comm, sizeof(e.comm), &task->comm);
    if (aegis_core_read(&parent, sizeof(parent), &task->real_parent) == 0 && parent) {
        aegis_core_read(&e.ppid, sizeof(e.ppid), &parent->tgid);
        aegis_core_read_str(e.parent_comm, sizeof(e.parent_comm), &parent->comm);
    }

    bpf_map_delete_elem(&guarded_pids, &tgid);

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
