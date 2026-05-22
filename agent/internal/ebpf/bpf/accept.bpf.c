//go:build ignore

#include "common.h"
#include "event_output.h"

// accept_event: layout-compatible with conn_event in tcp_connect.bpf.c.
// Changes here must be mirrored in tcp_connect.bpf.c and Go ConnEvent struct.
struct accept_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __u16 family;
    __u16 protocol;
    __u16 sport;
    __u16 dport;
    __s32 ret;
    __u8 comm[TASK_COMM_LEN];
    __u8 saddr[16];
    __u8 daddr[16];
};

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(accept_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(accept_events);
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct accept_event);
} accept_scratch SEC(".maps");
#endif

// Tracepoint on inet_sock_set_state: fires when a socket changes TCP state.
// We detect inbound connections by looking for TCP_SYN_RECV -> TCP_ESTABLISHED transitions.
SEC("tracepoint/sock/inet_sock_set_state")
int trace_inet_sock_set_state(struct trace_event_raw_inet_sock_set_state *ctx)
{
    // Only care about transitions to TCP_ESTABLISHED (1)
    if (ctx->newstate != 1)
        return 0;

    // Only care about transitions from TCP_SYN_RECV (3) - this is the inbound accept path
    // Outbound connections go from TCP_SYN_SENT (2) to TCP_ESTABLISHED (1)
    if (ctx->oldstate != 3)
        return 0;

    // Only care about TCP (protocol 6)
    if (ctx->protocol != 6)
        return 0;

    // Only care about AF_INET (2) and AF_INET6 (10)
    if (ctx->family != 2 && ctx->family != 10)
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 uid_gid = bpf_get_current_uid_gid();

#if defined(AEGIS_EVENT_RINGBUF)
    struct accept_event *e;
    e = bpf_ringbuf_reserve(&accept_events, sizeof(*e), 0);
    if (!e)
        return 0;
    __builtin_memset(e, 0, sizeof(*e));
#elif defined(AEGIS_EVENT_PERF)
    __u32 key = 0;
    struct accept_event *e = bpf_map_lookup_elem(&accept_scratch, &key);
    if (!e)
        return 0;
    __builtin_memset(e, 0, sizeof(*e));
#endif

    e->timestamp_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = pid_tgid & 0xFFFFFFFF;
    e->uid = uid_gid & 0xFFFFFFFF;
    e->gid = uid_gid >> 32;
    e->ret = 0;
    bpf_get_current_comm(e->comm, sizeof(e->comm));

    e->family = ctx->family;
    e->protocol = ctx->protocol;
    e->sport = ctx->sport;
    e->dport = ctx->dport;

    if (ctx->family == 2) { // AF_INET
        bpf_probe_read_kernel(e->saddr, 4, ctx->saddr);
        bpf_probe_read_kernel(e->daddr, 4, ctx->daddr);
    } else if (ctx->family == 10) { // AF_INET6
        bpf_probe_read_kernel(e->saddr, 16, ctx->saddr_v6);
        bpf_probe_read_kernel(e->daddr, 16, ctx->daddr_v6);
    }

#if defined(AEGIS_EVENT_RINGBUF)
    bpf_ringbuf_submit(e, 0);
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &accept_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
#endif
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
