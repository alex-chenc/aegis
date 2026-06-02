#ifndef __BPF_TRACING_H
#define __BPF_TRACING_H

#if defined(__TARGET_ARCH_x86)
#define PT_REGS_PARM1(ctx) ((ctx)->di)
#define PT_REGS_PARM2(ctx) ((ctx)->si)
#define PT_REGS_PARM3(ctx) ((ctx)->dx)
#define PT_REGS_PARM4(ctx) ((ctx)->cx)
#define PT_REGS_PARM5(ctx) ((ctx)->r8)
#define PT_REGS_RC(ctx) ((ctx)->ax)
#elif defined(__TARGET_ARCH_arm)
#define PT_REGS_PARM1(ctx) ((ctx)->regs[0])
#define PT_REGS_PARM2(ctx) ((ctx)->regs[1])
#define PT_REGS_PARM3(ctx) ((ctx)->regs[2])
#define PT_REGS_PARM4(ctx) ((ctx)->regs[3])
#define PT_REGS_PARM5(ctx) ((ctx)->regs[4])
#define PT_REGS_RC(ctx) ((ctx)->regs[0])
#else
#error "Define __TARGET_ARCH_x86 or __TARGET_ARCH_arm before including bpf_tracing.h"
#endif

#endif
