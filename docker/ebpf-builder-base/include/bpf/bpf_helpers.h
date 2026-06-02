#ifndef __BPF_HELPERS_H
#define __BPF_HELPERS_H

#include <linux/bpf.h>
#include <linux/types.h>

#ifndef __always_inline
#define __always_inline inline __attribute__((always_inline))
#endif

#ifndef NULL
#define NULL ((void *)0)
#endif

#ifndef SEC
#define SEC(name) __attribute__((section(name), used))
#endif

#ifndef __uint
#define __uint(name, val) int (*name)[val]
#endif

#ifndef __type
#define __type(name, val) typeof(val) *name
#endif

#ifndef __array
#define __array(name, val) typeof(val) *name[]
#endif

static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *)BPF_FUNC_map_lookup_elem;
static long (*bpf_map_update_elem)(void *map, const void *key, const void *value, __u64 flags) = (void *)BPF_FUNC_map_update_elem;
static long (*bpf_map_delete_elem)(void *map, const void *key) = (void *)BPF_FUNC_map_delete_elem;
static long (*bpf_probe_read_user)(void *dst, __u32 size, const void *unsafe_ptr) = (void *)BPF_FUNC_probe_read_user;
static long (*bpf_probe_read_user_str)(void *dst, __u32 size, const void *unsafe_ptr) = (void *)BPF_FUNC_probe_read_user_str;
static long (*bpf_probe_read_kernel)(void *dst, __u32 size, const void *unsafe_ptr) = (void *)BPF_FUNC_probe_read_kernel;
static long (*bpf_probe_read_kernel_str)(void *dst, __u32 size, const void *unsafe_ptr) = (void *)BPF_FUNC_probe_read_kernel_str;
static __u64 (*bpf_get_current_pid_tgid)(void) = (void *)BPF_FUNC_get_current_pid_tgid;
static __u64 (*bpf_get_current_uid_gid)(void) = (void *)BPF_FUNC_get_current_uid_gid;
static long (*bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *)BPF_FUNC_get_current_comm;
static __u64 (*bpf_ktime_get_ns)(void) = (void *)BPF_FUNC_ktime_get_ns;
static void *(*bpf_ringbuf_reserve)(void *ringbuf, __u64 size, __u64 flags) = (void *)BPF_FUNC_ringbuf_reserve;
static void (*bpf_ringbuf_submit)(void *data, __u64 flags) = (void *)BPF_FUNC_ringbuf_submit;
static void (*bpf_ringbuf_discard)(void *data, __u64 flags) = (void *)BPF_FUNC_ringbuf_discard;
static long (*bpf_perf_event_output)(void *ctx, void *map, __u64 flags, void *data, __u64 size) = (void *)BPF_FUNC_perf_event_output;

#endif
