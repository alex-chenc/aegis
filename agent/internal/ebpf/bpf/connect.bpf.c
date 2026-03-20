//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

#define TASK_COMM_LEN 16

struct conn_event {
    __u32 pid;
    __u32 uid;
    __u8 comm[TASK_COMM_LEN];
    __u8 saddr[4];
    __u8 daddr[4];
    __u16 sport;
    __u16 dport;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} conn_events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_connect")
int trace_connect(struct trace_event_raw_sys_enter *ctx)
{
    struct conn_event *e;
    struct sockaddr_in *sin;
    
    e = bpf_ringbuf_reserve(&conn_events, sizeof(*e), 0);
    if (!e)
        return 0;
    
    e->pid = bpf_get_current_pid_tgid() >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    bpf_get_current_comm(e->comm, sizeof(e->comm));
    
    sin = (struct sockaddr_in *)ctx->args[1];
    bpf_probe_read_user(&e->daddr, sizeof(e->daddr), &sin->sin_addr.s_addr);
    bpf_probe_read_user(&e->dport, sizeof(e->dport), &sin->sin_port);
    e->dport = bpf_ntohs(e->dport);
    
    bpf_ringbuf_submit(e, 0);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
