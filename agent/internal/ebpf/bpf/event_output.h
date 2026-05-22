#ifndef __AEGIS_EVENT_OUTPUT_H
#define __AEGIS_EVENT_OUTPUT_H

#include "common.h"

#if defined(AEGIS_EVENT_RINGBUF)

#define DEFINE_RINGBUF_MAP(name) \
    struct { \
        __uint(type, BPF_MAP_TYPE_RINGBUF); \
        __uint(max_entries, 256 * 1024); \
    } name SEC(".maps")

#define EVENT_RESERVE(map, st) \
    bpf_ringbuf_reserve(&map, sizeof(st), 0)

#define EVENT_SUBMIT(event) \
    bpf_ringbuf_submit(event, 0)

#define EVENT_DISCARD(event) \
    bpf_ringbuf_discard(event, 0)

#elif defined(AEGIS_EVENT_PERF)

#define DEFINE_PERF_MAP(name) \
    struct { \
        __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY); \
        __uint(key_size, sizeof(__u32)); \
        __uint(value_size, sizeof(__u32)); \
    } name SEC(".maps")

#define EVENT_DISCARD(event) ((void)0)

#endif

#endif
