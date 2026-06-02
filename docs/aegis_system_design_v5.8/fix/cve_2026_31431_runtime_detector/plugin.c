#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

typedef __u8 u8;
typedef __u16 u16;
typedef __u32 u32;
typedef __u64 u64;

#define AEGIS_PLUGIN_PAYLOAD_MAX 256
#define AF_ALG 38

struct trace_event_raw_sys_enter {
	unsigned short common_type;
	unsigned char common_flags;
	unsigned char common_preempt_count;
	int common_pid;
	long id;
	unsigned long args[6];
};

struct sockaddr_alg_min {
	u16 salg_family;
	u8 salg_type[14];
	u32 salg_feat;
	u32 salg_mask;
	u8 salg_name[64];
};

struct aegis_plugin_event {
	u64 timestamp_ns;
	u32 plugin_id_hash;
	u32 event_type;
	u32 pid;
	u32 tid;
	u32 uid;
	u32 gid;
	u32 payload_len;
	u8 payload[AEGIS_PLUGIN_PAYLOAD_MAX];
};

#if defined(AEGIS_EVENT_RINGBUF)
struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 20);
} events SEC(".maps");
#elif defined(AEGIS_EVENT_PERF)
struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
	__uint(key_size, sizeof(u32));
	__uint(value_size, sizeof(u32));
} events SEC(".maps");
#endif

static __always_inline void fill_common(struct aegis_plugin_event *e, u32 event_type)
{
	u64 pid_tgid = bpf_get_current_pid_tgid();
	u64 uid_gid = bpf_get_current_uid_gid();

	e->timestamp_ns = bpf_ktime_get_ns();
	e->plugin_id_hash = 0xb1c4300a;
	e->event_type = event_type;
	e->pid = (u32)(pid_tgid >> 32);
	e->tid = (u32)pid_tgid;
	e->uid = (u32)uid_gid;
	e->gid = (u32)(uid_gid >> 32);
	e->payload_len = 0;
}

static __always_inline int emit_event(void *ctx, u32 event_type)
{
#if defined(AEGIS_EVENT_RINGBUF)
	struct aegis_plugin_event *e;

	e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
	if (!e)
		return 0;
	fill_common(e, event_type);
	bpf_ringbuf_submit(e, 0);
#elif defined(AEGIS_EVENT_PERF)
	struct aegis_plugin_event e = {};

	fill_common(&e, event_type);
	bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
#endif
	return 0;
}

SEC("tracepoint/syscalls/sys_enter_socket")
int tracepoint__syscalls__sys_enter_socket(struct trace_event_raw_sys_enter *ctx)
{
	u32 family = (u32)ctx->args[0];

	if (family != AF_ALG)
		return 0;
	return emit_event(ctx, 1001);
}

SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__syscalls__sys_enter_bind(struct trace_event_raw_sys_enter *ctx)
{
	struct sockaddr_alg_min sa = {};
	void *sockaddr_ptr = (void *)ctx->args[1];

	if (!sockaddr_ptr)
		return 0;
	if (bpf_probe_read_user(&sa, sizeof(sa), sockaddr_ptr) < 0)
		return 0;
	if (sa.salg_family != AF_ALG)
		return 0;
	return emit_event(ctx, 1002);
}

SEC("tracepoint/syscalls/sys_enter_splice")
int tracepoint__syscalls__sys_enter_splice(struct trace_event_raw_sys_enter *ctx)
{
	return emit_event(ctx, 1003);
}

char LICENSE[] SEC("license") = "GPL";
