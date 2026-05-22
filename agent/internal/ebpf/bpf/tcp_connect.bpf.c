//go:build ignore

#include "common.h"
#include "event_output.h"

// conn_event: layout-compatible with accept_event in accept.bpf.c.
// Changes here must be mirrored in accept.bpf.c and Go ConnEvent struct.
struct conn_event {
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

struct pending_conn {
    __u16 family;
    __u16 protocol;
    __u16 dport;
    __u8 daddr[16];
};

// Map to store connect arguments between entry and return
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u64);
    __type(value, struct pending_conn);
} conn_map SEC(".maps");

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(conn_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(conn_events);
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct conn_event);
} conn_scratch SEC(".maps");
#endif

// Common entry handler for tcp_v4_connect and tcp_v6_connect.
// Destination address is stable in the function arguments; reading it here
// avoids relying on kernel-internal struct sock layout differences.
static __always_inline int tcp_connect_entry(void *uaddr, __u16 family)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct pending_conn conn = {};
    conn.family = family;
    conn.protocol = 6; // TCP

    if (family == 2) { // AF_INET
        struct sockaddr_in *sin = (struct sockaddr_in *)uaddr;
        __be32 daddr = 0;
        __be16 dport = 0;
        bpf_probe_read_kernel(&daddr, sizeof(daddr), &sin->sin_addr.s_addr);
        bpf_probe_read_kernel(&dport, sizeof(dport), &sin->sin_port);
        __builtin_memcpy(conn.daddr, &daddr, 4);
        conn.dport = __bpf_ntohs(dport);
    } else if (family == 10) { // AF_INET6
        struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *)uaddr;
        __be16 dport = 0;
        bpf_probe_read_kernel(conn.daddr, 16, &sin6->sin6_addr.in6_u.u6_addr8);
        bpf_probe_read_kernel(&dport, sizeof(dport), &sin6->sin6_port);
        conn.dport = __bpf_ntohs(dport);
    } else {
        return 0;
    }

    bpf_map_update_elem(&conn_map, &pid_tgid, &conn, BPF_ANY);
    return 0;
}

// Common return handler
static __always_inline int tcp_connect_return(struct pt_regs *ctx, __s32 ret)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct pending_conn *conn = bpf_map_lookup_elem(&conn_map, &pid_tgid);
    if (!conn) {
        return 0;
    }
    struct pending_conn pending = {};
    __builtin_memcpy(&pending, conn, sizeof(pending));
    bpf_map_delete_elem(&conn_map, &pid_tgid);

#if defined(AEGIS_EVENT_RINGBUF)
    struct conn_event *e;
    e = bpf_ringbuf_reserve(&conn_events, sizeof(*e), 0);
    if (!e)
        return 0;
    __builtin_memset(e, 0, sizeof(*e));
#elif defined(AEGIS_EVENT_PERF)
    __u32 key = 0;
    struct conn_event *e = bpf_map_lookup_elem(&conn_scratch, &key);
    if (!e)
        return 0;
    __builtin_memset(e, 0, sizeof(*e));
#endif

    e->timestamp_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = pid_tgid & 0xFFFFFFFF;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->gid = bpf_get_current_uid_gid() >> 32;
    e->ret = ret;
    bpf_get_current_comm(e->comm, sizeof(e->comm));

    e->family = pending.family;
    e->protocol = pending.protocol;
    e->dport = pending.dport;
    __builtin_memcpy(e->daddr, pending.daddr, sizeof(e->daddr));

#if defined(AEGIS_EVENT_RINGBUF)
    bpf_ringbuf_submit(e, 0);
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &conn_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
#endif
    return 0;
}

// kprobe entry points
SEC("kprobe/tcp_v4_connect")
int kprobe_tcp_v4_connect(struct pt_regs *ctx)
{
    void *uaddr = (void *)PT_REGS_PARM2(ctx);
    return tcp_connect_entry(uaddr, 2);
}

SEC("kprobe/tcp_v6_connect")
int kprobe_tcp_v6_connect(struct pt_regs *ctx)
{
    void *uaddr = (void *)PT_REGS_PARM2(ctx);
    return tcp_connect_entry(uaddr, 10);
}

// kretprobe return points
SEC("kretprobe/tcp_v4_connect")
int kretprobe_tcp_v4_connect(struct pt_regs *ctx)
{
    __s32 ret = (__s32)PT_REGS_RC(ctx);
    return tcp_connect_return(ctx, ret);
}

SEC("kretprobe/tcp_v6_connect")
int kretprobe_tcp_v6_connect(struct pt_regs *ctx)
{
    __s32 ret = (__s32)PT_REGS_RC(ctx);
    return tcp_connect_return(ctx, ret);
}

char LICENSE[] SEC("license") = "GPL";
