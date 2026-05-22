/* Minimal vmlinux.h stub for eBPF programs */
#ifndef __VMLINUX_H__
#define __VMLINUX_H__

#include <linux/types.h>
#include <linux/bpf.h>

/* Task struct stub for BPF_CORE_READ */
struct task_struct {
    struct task_struct *real_parent;
    int tgid;
    char comm[16];  /* TASK_COMM_LEN */
};

/* Tracepoint context for sys_enter */
struct trace_event_raw_sys_enter {
    __u64 unused;
    __u64 syscall_nr;
    __u64 args[6];
};

/* Tracepoint context for sys_exit */
struct trace_event_raw_sys_exit {
    __u64 unused;
    __u64 syscall_nr;
    __s64 ret;
};

/* Tracepoint context for inet_sock_set_state */
struct trace_event_raw_inet_sock_set_state {
    __u64 unused;
    const void *skaddr;
    int oldstate;
    int newstate;
    __u16 sport;
    __u16 dport;
    __u16 family;
    __u16 protocol;
    __u8 saddr[4];
    __u8 daddr[4];
    __u8 saddr_v6[16];
    __u8 daddr_v6[16];
};

/* Socket address structures */
struct sockaddr {
    unsigned short sa_family;
    char sa_data[14];
};

struct sockaddr_in {
    unsigned short sin_family;
    unsigned short sin_port;
    struct {
        unsigned int s_addr;
    } sin_addr;
    char sin_zero[8];
};

struct in6_addr {
    union {
        __u8 u6_addr8[16];
        __be16 u6_addr16[8];
        __be32 u6_addr32[4];
    } in6_u;
};

struct sockaddr_in6 {
    unsigned short sin6_family;
    unsigned short sin6_port;
    __u32 sin6_flowinfo;
    struct in6_addr sin6_addr;
    __u32 sin6_scope_id;
};

#endif /* __VMLINUX_H__ */
