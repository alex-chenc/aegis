#ifndef __AEGIS_COMMON_H
#define __AEGIS_COMMON_H

// Architecture define required for PT_REGS macros in bpf_tracing.h
#if !defined(__TARGET_ARCH_x86) && !defined(__TARGET_ARCH_arm)
#define __TARGET_ARCH_x86 1
#endif

#include "vmlinux.h"

// user_pt_regs is needed by bpf_tracing.h PT_REGS macros when __VMLINUX_H__ is defined
// but vmlinux.h may not define it. Provide x86_64 definition here.
#if defined(__TARGET_ARCH_x86) || defined(__x86_64__)
#ifndef _UAPI_ASM_X86_PTRACE_H
struct user_pt_regs {
    unsigned long r15;
    unsigned long r14;
    unsigned long r13;
    unsigned long r12;
    unsigned long bp;
    unsigned long bx;
    unsigned long r11;
    unsigned long r10;
    unsigned long r9;
    unsigned long r8;
    unsigned long ax;
    unsigned long cx;
    unsigned long dx;
    unsigned long si;
    unsigned long di;
    unsigned long orig_ax;
    unsigned long ip;
    unsigned long cs;
    unsigned long flags;
    unsigned long sp;
    unsigned long ss;
};
#define _UAPI_ASM_X86_PTRACE_H
#endif
#endif

// struct pt_regs with short field names for kernel-side PT_REGS macros
#if defined(__KERNEL__) || defined(__VMLINUX_H__)
#ifndef _AEGIS_PT_REGS_DEFINED
struct pt_regs {
    unsigned long r15;
    unsigned long r14;
    unsigned long r13;
    unsigned long r12;
    unsigned long bp;
    unsigned long bx;
    unsigned long r11;
    unsigned long r10;
    unsigned long r9;
    unsigned long r8;
    unsigned long ax;
    unsigned long cx;
    unsigned long dx;
    unsigned long si;
    unsigned long di;
    unsigned long orig_ax;
    unsigned long ip;
    unsigned long cs;
    unsigned long flags;
    unsigned long sp;
    unsigned long ss;
};
#define _AEGIS_PT_REGS_DEFINED
#endif
#endif

#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

#define TASK_COMM_LEN 16
#define MAX_PATH_LEN 256
#define MAX_ARGS_LEN 512
#define MAX_ARGC 8
#define MAX_SINGLE_ARG_LEN 64
#define EXEC_EVENT_ARGS_TRUNCATED (1U << 0)

// File action codes
#define FILE_ACTION_OPEN_WRITE  1
#define FILE_ACTION_CREATE      2
#define FILE_ACTION_TRUNCATE    3
#define FILE_ACTION_DELETE      4
#define FILE_ACTION_RENAME      5
#define FILE_ACTION_CHMOD       6
#define FILE_ACTION_CHOWN       7
#define FILE_ACTION_OPEN_READ   8

// Write intent flags
#define O_WRONLY     00000001
#define O_RDWR       00000002
#define O_CREAT      00000100
#define O_TRUNC      00001000
#define O_APPEND     00002000

struct open_how {
    __u64 flags;
    __u64 mode;
    __u64 resolve;
};

// Minimal sock definitions for CO-RE access to kernel struct sock
struct sock_common {
    union {
        struct {
            __be32 skc_daddr;
            __be32 skc_rcv_saddr;
        };
    };
    union {
        struct {
            __be16 skc_dport;
            __u16 skc_num;
        };
    };
    unsigned short skc_family;
    volatile unsigned char skc_state;
    __u8 skc_bound_dev_if;
    union {
        __be32 skc_v6_daddr[4];
    };
    __u8 _pad[12];
    union {
        __be32 skc_v6_rcv_saddr[4];
    };
};

struct sock {
    struct sock_common __sk_common;
    __u8 sk_protocol;
};

#endif
