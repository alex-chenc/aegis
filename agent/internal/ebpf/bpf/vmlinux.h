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
    __u64 args[6];
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

#endif /* __VMLINUX_H__ */
