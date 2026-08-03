//go:build ignore

#include "common.h"
#include "event_output.h"

#define AG_ACTION_DENY            1
#define AG_ACTION_DENY_AND_FREEZE 2

#define AG_OP_EXEC       5
#define AG_OP_CONNECT    6
#define AG_OP_SETNS      7
#define AG_OP_MOUNT      8
#define AG_OP_PTRACE     9
#define AG_OP_LOAD      10

#define AG_ESCAPE_BPF    0

struct socket;
struct path;
union bpf_attr;

struct guard_subject {
    __u64 instance_slot;
    __u64 unit_slot;
    __u64 policy_slot;
    __u64 process_epoch;
    __u32 flags;
    __u32 pad;
};

struct policy_path_key {
    __u32 prefixlen;
    __u8 data[sizeof(__u64) + MAX_PATH_LEN];
};

struct path_rule_value {
    __u64 rule_slot;
    __u32 operation_mask;
    __u32 action;
    __u32 match_kind;
    __u32 path_len;
};

struct escape_policy_value {
    __u32 action_by_rule[8];
    __u64 rule_slot_by_rule[8];
};

struct agent_guard_lsm_event {
    __u64 timestamp_ns;
    __u64 instance_slot;
    __u64 unit_slot;
    __u64 policy_slot;
    __u64 rule_slot;
    __u32 pid;
    __u32 uid;
    __u32 operation;
    __u32 action;
    __u8 comm[TASK_COMM_LEN];
    __u8 resource[MAX_PATH_LEN];
};

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

struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __uint(max_entries, 8192);
    __type(key, struct policy_path_key);
    __type(value, struct path_rule_value);
} path_rules SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 8192);
    __type(key, __u64);
    __type(value, struct escape_policy_value);
} unit_escape_policies SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct agent_guard_lsm_event);
} agent_guard_lsm_scratch SEC(".maps");

static __u64 (*agent_guard_get_current_cgroup_id)(void) =
    (void *)BPF_FUNC_get_current_cgroup_id;

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(agent_guard_lsm_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(agent_guard_lsm_events);
#endif

static __always_inline struct guard_subject *current_subject(void)
{
    __u32 tgid = bpf_get_current_pid_tgid() >> 32;
    struct guard_subject *subject = bpf_map_lookup_elem(&guarded_pids, &tgid);
    if (subject)
        return subject;
    __u64 cgroup_id = agent_guard_get_current_cgroup_id();
    return bpf_map_lookup_elem(&guarded_cgroups, &cgroup_id);
}

static __always_inline int emit_deny(
    void *ctx, const struct guard_subject *subject, __u64 rule_slot,
    __u32 operation, __u32 action, const char *resource)
{
    __u32 zero = 0;
    struct agent_guard_lsm_event *event =
        bpf_map_lookup_elem(&agent_guard_lsm_scratch, &zero);
    if (!event)
        return -1;
    __builtin_memset(event, 0, sizeof(*event));
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    event->timestamp_ns = bpf_ktime_get_ns();
    event->pid = pid_tgid >> 32;
    event->uid = (__u32)bpf_get_current_uid_gid();
    event->instance_slot = subject->instance_slot;
    event->unit_slot = subject->unit_slot;
    event->policy_slot = subject->policy_slot;
    event->rule_slot = rule_slot;
    event->operation = operation;
    event->action = action;
    bpf_get_current_comm(event->comm, sizeof(event->comm));
    if (resource)
        bpf_probe_read_kernel_str(event->resource, sizeof(event->resource), resource);
#if defined(AEGIS_EVENT_RINGBUF)
    struct agent_guard_lsm_event *output =
        bpf_ringbuf_reserve(&agent_guard_lsm_events, sizeof(*output), 0);
    if (output) {
        __builtin_memcpy(output, event, sizeof(*output));
        bpf_ringbuf_submit(output, 0);
    }
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &agent_guard_lsm_events, BPF_F_CURRENT_CPU,
                          event, sizeof(*event));
#endif
    return -1;
}

static __always_inline int apply_path_rule(
    void *ctx, const struct guard_subject *subject, const char *path,
    __u32 operation)
{
    __u32 zero = 0;
    struct agent_guard_lsm_event *scratch =
        bpf_map_lookup_elem(&agent_guard_lsm_scratch, &zero);
    if (!scratch || !path)
        return 0;
    __builtin_memset(scratch->resource, 0, sizeof(scratch->resource));
    int length = bpf_probe_read_kernel_str(scratch->resource,
                                           sizeof(scratch->resource), path);
    if (length <= 1)
        return 0;
    struct policy_path_key key = {};
    __builtin_memcpy(key.data, &subject->policy_slot, sizeof(subject->policy_slot));
    __builtin_memcpy(key.data + sizeof(subject->policy_slot),
                     scratch->resource, sizeof(scratch->resource));
    key.prefixlen = 64 + (__u32)(length - 1) * 8;
    struct path_rule_value *rule = bpf_map_lookup_elem(&path_rules, &key);
    if (!rule || !(rule->operation_mask & (1U << operation)) || !rule->action)
        return 0;
    if (rule->match_kind == 1 && rule->path_len != (__u32)(length - 1))
        return 0;
    return emit_deny(ctx, subject, rule->rule_slot, operation, rule->action,
                     (const char *)scratch->resource);
}

static __always_inline int apply_escape_rule(
    void *ctx, const struct guard_subject *subject, __u32 index,
    __u32 operation)
{
    struct escape_policy_value *policy =
        bpf_map_lookup_elem(&unit_escape_policies, &subject->unit_slot);
    if (!policy || index >= 8 || !policy->action_by_rule[index])
        return 0;
    return emit_deny(ctx, subject, policy->rule_slot_by_rule[index], operation,
                     policy->action_by_rule[index], NULL);
}

SEC("lsm/socket_connect")
int agent_guard_socket_connect(unsigned long long *ctx)
{
    struct sockaddr *address = (struct sockaddr *)ctx[1];
    int addrlen = (int)ctx[2];
    struct guard_subject *subject = current_subject();
    if (!subject || !address || addrlen < 3)
        return 0;
    unsigned short family = 0;
    bpf_probe_read_kernel(&family, sizeof(family), &address->sa_family);
    if (family != 1)
        return 0;
    return apply_path_rule(ctx, subject, address->sa_data, AG_OP_CONNECT);
}

SEC("lsm/bpf")
int agent_guard_bpf(unsigned long long *ctx)
{
    struct guard_subject *subject = current_subject();
    if (!subject)
        return 0;
    int cmd = (int)ctx[0];
    if (cmd != BPF_PROG_LOAD && cmd != BPF_BTF_LOAD)
        return 0;
    return apply_escape_rule(ctx, subject, AG_ESCAPE_BPF, AG_OP_LOAD);
}

char LICENSE[] SEC("license") = "GPL";
