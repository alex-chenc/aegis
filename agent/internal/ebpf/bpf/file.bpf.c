//go:build ignore

#include "common.h"
#include "event_output.h"

struct file_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __s32 flags;
    __s32 ret;
    __u32 action;
    __u8 comm[TASK_COMM_LEN];
    __u8 path[MAX_PATH_LEN];
    __u8 old_path[MAX_PATH_LEN];
};

#if defined(AEGIS_EVENT_RINGBUF)
DEFINE_RINGBUF_MAP(file_events);
#elif defined(AEGIS_EVENT_PERF)
DEFINE_PERF_MAP(file_events);
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct file_event);
} file_scratch SEC(".maps");
#endif

// Check if open flags indicate write intent
static __always_inline int is_write_intent(__s32 flags)
{
    return (flags & (O_WRONLY | O_RDWR | O_CREAT | O_TRUNC | O_APPEND)) != 0;
}

static __always_inline int starts_with(const __u8 *path, const char *prefix, int len)
{
#pragma unroll
    for (int i = 0; i < 24; i++) {
        if (i >= len)
            return 1;
        if (path[i] != prefix[i])
            return 0;
    }
    return 1;
}

static __always_inline int is_sensitive_path(const __u8 *path)
{
    if (path[0] != '/')
        return 0;

    if (starts_with(path, "/etc/", 5)) return 1;
    if (starts_with(path, "/root/", 6)) return 1;
    if (starts_with(path, "/boot/", 6)) return 1;
    if (starts_with(path, "/usr/bin/", 9)) return 1;
    if (starts_with(path, "/usr/sbin/", 10)) return 1;
    if (starts_with(path, "/bin/", 5)) return 1;
    if (starts_with(path, "/sbin/", 6)) return 1;
    if (starts_with(path, "/lib/systemd/", 13)) return 1;
    if (starts_with(path, "/etc/systemd/", 13)) return 1;
    if (starts_with(path, "/var/spool/cron/", 16)) return 1;
    if (starts_with(path, "/tmp/", 5)) return 1;
    if (starts_with(path, "/var/tmp/", 9)) return 1;
    if (starts_with(path, "/sys/fs/cgroup/", 15)) return 1;
    if (starts_with(path, "/run/", 5)) return 1;
    if (starts_with(path, "/var/run/", 9)) return 1;

    return 0;
}

// Agent self-PID exclusion. The userspace loader writes the agent's own tgid
// here so file operations performed by the agent itself - notably /proc reads
// done while enriching events - are dropped in-kernel instead of feeding a
// self-monitoring feedback loop.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} agent_pid_config SEC(".maps");

static __always_inline int is_agent_self(void)
{
    __u32 key = 0;
    __u32 *agent_pid = bpf_map_lookup_elem(&agent_pid_config, &key);
    if (agent_pid && *agent_pid != 0 && (bpf_get_current_pid_tgid() >> 32) == *agent_pid)
        return 1;
    return 0;
}

// Submit a file event helper
static __always_inline void submit_file_event(
    struct trace_event_raw_sys_enter *ctx,
    __u32 action,
    __s32 flags,
    const char *path_ptr,
    const char *old_path_ptr)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();

#if defined(AEGIS_EVENT_RINGBUF)
    struct file_event *e;
    e = bpf_ringbuf_reserve(&file_events, sizeof(*e), 0);
    if (!e)
        return;
    __builtin_memset(e, 0, sizeof(*e));
#elif defined(AEGIS_EVENT_PERF)
    __u32 key = 0;
    struct file_event *e = bpf_map_lookup_elem(&file_scratch, &key);
    if (!e)
        return;
    __builtin_memset(e, 0, sizeof(*e));
#endif

    e->timestamp_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = pid_tgid & 0xFFFFFFFF;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->gid = bpf_get_current_uid_gid() >> 32;
    e->flags = flags;
    e->ret = 0;
    e->action = action;
    bpf_get_current_comm(e->comm, sizeof(e->comm));

    if (path_ptr)
        bpf_probe_read_user_str(e->path, sizeof(e->path), path_ptr);
    if (old_path_ptr)
        bpf_probe_read_user_str(e->old_path, sizeof(e->old_path), old_path_ptr);

    if (!is_sensitive_path(e->path)) {
#if defined(AEGIS_EVENT_RINGBUF)
        bpf_ringbuf_discard(e, 0);
#endif
        return;
    }

#if defined(AEGIS_EVENT_RINGBUF)
    bpf_ringbuf_submit(e, 0);
#elif defined(AEGIS_EVENT_PERF)
    bpf_perf_event_output(ctx, &file_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
#endif
}

// openat: int openat(int dirfd, const char *pathname, int flags, ...)
SEC("tracepoint/syscalls/sys_enter_openat")
int trace_openat(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    __s32 flags = (__s32)ctx->args[2];
    const char *pathname = (const char *)ctx->args[1];
    if (!pathname)
        return 0;

    // Quick path check - only collect events for absolute paths
    char buf[32];
    if (bpf_probe_read_user_str(buf, sizeof(buf), pathname) < 0)
        return 0;
    if (buf[0] != '/')
        return 0;
    if (!is_write_intent(flags) && !is_sensitive_path((const __u8 *)buf))
        return 0;

    __u32 action = is_write_intent(flags) ? FILE_ACTION_OPEN_WRITE : FILE_ACTION_OPEN_READ;
    if (flags & O_CREAT)
        action = FILE_ACTION_CREATE;
    if (flags & O_TRUNC)
        action = FILE_ACTION_TRUNCATE;

    submit_file_event(ctx, action, flags, pathname, NULL);
    return 0;
}

// openat2 (same as openat for our purposes)
SEC("tracepoint/syscalls/sys_enter_openat2")
int trace_openat2(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    struct open_how how = {};
    struct open_how *how_ptr = (struct open_how *)ctx->args[2];
    if (!how_ptr)
        return 0;
    if (bpf_probe_read_user(&how, sizeof(how), how_ptr) < 0)
        return 0;

    __s32 flags = (__s32)how.flags;
    const char *pathname = (const char *)ctx->args[1];
    if (!pathname)
        return 0;

    char buf[32];
    if (bpf_probe_read_user_str(buf, sizeof(buf), pathname) < 0)
        return 0;
    if (buf[0] != '/')
        return 0;
    if (!is_write_intent(flags) && !is_sensitive_path((const __u8 *)buf))
        return 0;

    __u32 action = is_write_intent(flags) ? FILE_ACTION_OPEN_WRITE : FILE_ACTION_OPEN_READ;
    if (flags & O_CREAT)
        action = FILE_ACTION_CREATE;
    if (flags & O_TRUNC)
        action = FILE_ACTION_TRUNCATE;

    submit_file_event(ctx, action, flags, pathname, NULL);
    return 0;
}

// creat: int creat(const char *pathname, mode_t mode)
SEC("tracepoint/syscalls/sys_enter_creat")
int trace_creat(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *pathname = (const char *)ctx->args[0];
    submit_file_event(ctx, FILE_ACTION_CREATE, O_CREAT, pathname, NULL);
    return 0;
}

// unlinkat: int unlinkat(int dirfd, const char *pathname, int flags)
SEC("tracepoint/syscalls/sys_enter_unlinkat")
int trace_unlinkat(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *pathname = (const char *)ctx->args[1];
    submit_file_event(ctx, FILE_ACTION_DELETE, 0, pathname, NULL);
    return 0;
}

// renameat: int renameat(int olddirfd, const char *oldpath, int newdirfd, const char *newpath)
SEC("tracepoint/syscalls/sys_enter_renameat")
int trace_renameat(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *oldpath = (const char *)ctx->args[1];
    const char *newpath = (const char *)ctx->args[3];
    submit_file_event(ctx, FILE_ACTION_RENAME, 0, newpath, oldpath);
    return 0;
}

// renameat2: int renameat2(int olddirfd, const char *oldpath, int newdirfd, const char *newpath, unsigned int flags)
SEC("tracepoint/syscalls/sys_enter_renameat2")
int trace_renameat2(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *oldpath = (const char *)ctx->args[1];
    const char *newpath = (const char *)ctx->args[3];
    submit_file_event(ctx, FILE_ACTION_RENAME, 0, newpath, oldpath);
    return 0;
}

// chmod: int chmod(const char *pathname, mode_t mode)
SEC("tracepoint/syscalls/sys_enter_chmod")
int trace_chmod(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *pathname = (const char *)ctx->args[0];
    submit_file_event(ctx, FILE_ACTION_CHMOD, 0, pathname, NULL);
    return 0;
}

// fchmodat: int fchmodat(int dirfd, const char *pathname, mode_t mode, int flags)
SEC("tracepoint/syscalls/sys_enter_fchmodat")
int trace_fchmodat(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *pathname = (const char *)ctx->args[1];
    submit_file_event(ctx, FILE_ACTION_CHMOD, 0, pathname, NULL);
    return 0;
}

// chown: int chown(const char *pathname, uid_t owner, gid_t group)
SEC("tracepoint/syscalls/sys_enter_chown")
int trace_chown(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *pathname = (const char *)ctx->args[0];
    submit_file_event(ctx, FILE_ACTION_CHOWN, 0, pathname, NULL);
    return 0;
}

// fchownat: int fchownat(int dirfd, const char *pathname, uid_t owner, gid_t group, int flags)
SEC("tracepoint/syscalls/sys_enter_fchownat")
int trace_fchownat(struct trace_event_raw_sys_enter *ctx)
{
    if (is_agent_self())
        return 0;

    const char *pathname = (const char *)ctx->args[1];
    submit_file_event(ctx, FILE_ACTION_CHOWN, 0, pathname, NULL);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
