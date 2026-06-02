#ifndef __BPF_ENDIAN_H
#define __BPF_ENDIAN_H

#include <linux/types.h>

#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
#define __bpf_ntohs(x) __builtin_bswap16(x)
#define __bpf_htons(x) __builtin_bswap16(x)
#define __bpf_ntohl(x) __builtin_bswap32(x)
#define __bpf_htonl(x) __builtin_bswap32(x)
#else
#define __bpf_ntohs(x) (x)
#define __bpf_htons(x) (x)
#define __bpf_ntohl(x) (x)
#define __bpf_htonl(x) (x)
#endif

#define bpf_ntohs(x) __bpf_ntohs(x)
#define bpf_htons(x) __bpf_htons(x)
#define bpf_ntohl(x) __bpf_ntohl(x)
#define bpf_htonl(x) __bpf_htonl(x)

#endif
