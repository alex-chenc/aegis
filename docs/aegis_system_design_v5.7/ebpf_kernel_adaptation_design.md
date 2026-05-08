# V5.7 eBPF内核版本适配设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 现状分析

| 问题 | 影响 |
|:---|:---|
| 只使用ringbuf（需kernel 5.8+） | 4.18-5.7内核无法使用eBPF |
| 自定义vmlinux.h是手写stub | 结构体可能与实际内核不匹配 |
| 无BTF CO-RE适配 | 不同内核结构体布局可能不同 |
| 无内核版本检测 | 无法动态选择方案 |
| exit程序未列入Makefile | exit.bpf.o不会被构建 |

### 1.1 内核版本分布（企业环境典型）

| 内核版本 | 占比 | ringbuf | BTF | 推荐方案 |
|:---|:---|:---|:---|:---|
| 5.8+ | ~60% | 支持 | 支持 | CO-RE + ringbuf |
| 5.4-5.7 | ~20% | 不支持 | 支持 | CO-RE + perf buffer |
| 4.18-5.3 | ~15% | 不支持 | 部分 | 非CO-RE + perf buffer |
| < 4.18 | ~5% | 不支持 | 不支持 | /proc轮询 |

---

## 2. 内核能力检测

### 2.1 检测模块

```go
// kernel/detector.go
type KernelCapabilities struct {
    KernelVersion    string
    Major, Minor     int
    BTFAvailable     bool
    RingbufAvailable bool
    PerfBufferOnly   bool
    ProcFallback     bool
}

func Detect() (*KernelCapabilities, error) {
    caps := &KernelCapabilities{}

    // 1. 读取内核版本
    caps.Major, caps.Minor = readKernelVersion()

    // 2. 检测BTF
    caps.BTFAvailable = features.HaveBTF() == nil

    // 3. 检测ringbuf
    caps.RingbufAvailable = features.HaveMapType(ebpf.RingBuf) == nil

    // 4. 确定方案
    if caps.RingbufAvailable && caps.BTFAvailable {
        // 最优: CO-RE + ringbuf
    } else if caps.BTFAvailable {
        caps.PerfBufferOnly = true
    } else if caps.Major > 4 || (caps.Major == 4 && caps.Minor >= 18) {
        caps.PerfBufferOnly = true
    } else {
        caps.ProcFallback = true
    }
    return caps, nil
}
```

### 2.2 启动日志

```
[eBPF] 内核能力检测:
  版本: 5.10.0-1160.el7.x86_64
  BTF: 可用
  Ringbuf: 可用
  方案: CO-RE + ringbuf (最优)
  程序: execve, fork, openat, connect
```

---

## 3. 三级降级策略

```
Agent启动 → kernel.Detect()
    │
    ├─ ProcFallback ──→ /proc轮询（每秒扫描/proc）
    │
    ├─ PerfBufferOnly → perf buffer模式
    │   ├── BTF可用: CO-RE编译产物
    │   └── BTF不可用: 非CO-RE编译产物
    │
    └─ RingbufAvailable → ringbuf模式（最优）
        └── CO-RE编译产物
```

### 3.1 统一事件读取接口

```go
type EventReader interface {
    Read() ([]byte, error)
    Close() error
}

type RingbufReader struct { reader *ringbuf.Reader }
func (r *RingbufReader) Read() ([]byte, error) {
    rec, err := r.reader.Read()
    return rec.RawSample, err
}

type PerfReader struct { reader *perf.Reader }
func (r *PerfReader) Read() ([]byte, error) {
    rec, err := r.reader.Read()
    return rec.RawSample, err
}
```

### 3.2 Loader改造

```go
func (l *Loader) LoadAll() error {
    caps, _ := kernel.Detect()
    l.caps = caps

    if caps.ProcFallback {
        return ErrProcFallback
    }

    for _, name := range []string{"execve", "fork", "openat", "connect"} {
        objPath := l.selectObject(name, caps)
        coll, err := l.loadCollection(objPath)
        if err != nil {
            continue
        }
        if caps.RingbufAvailable {
            l.setupRingbufReader(name, coll)
        } else {
            l.setupPerfReader(name, coll)
        }
    }
    return nil
}

func (l *Loader) selectObject(name string, caps *kernel.KernelCapabilities) string {
    if caps.BTFAvailable {
        return fmt.Sprintf("bpf/obj/%s.bpf.o", name)
    }
    return fmt.Sprintf("bpf/obj/%s.noncore.bpf.o", name)
}
```

---

## 4. 编译产物管理

### 4.1 Makefile改造

```makefile
BPF_PROGRAMS = execve fork openat connect setuid setgid capset exit

# CO-RE编译
bpf-core:
	@for prog in $(BPF_PROGRAMS); do \
		clang -target bpf -D__TARGET_ARCH_$(ARCH) -O2 -g \
			-c bpf/$${prog}.bpf.c -o bpf/obj/$${prog}.bpf.o; \
	done

# 非CO-RE编译
bpf-noncore:
	@for prog in $(BPF_PROGRAMS); do \
		clang -target bpf -D__TARGET_ARCH_$(ARCH) -DCO_RE=0 -O2 -g \
			-c bpf/$${prog}.bpf.c -o bpf/obj/$${prog}.noncore.bpf.o; \
	done

# 两套都编译
bpf-all: bpf-core bpf-noncore
```

### 4.2 目录结构

```
bpf/obj/
├── execve.bpf.o           # CO-RE
├── execve.noncore.bpf.o   # 非CO-RE
├── fork.bpf.o
├── fork.noncore.bpf.o
├── openat.bpf.o
├── openat.noncore.bpf.o
├── connect.bpf.o
├── connect.noncore.bpf.o
└── ...
```

---

## 5. vmlinux.h管理

**V5.7方案**: 继续使用手写stub，但扩展定义以覆盖 `sockaddr_in6` 等新增结构体。

**后续版本**: 维护多版本vmlinux.h（按目标内核版本选择）。

---

## 6. 运行时监控

```go
type EBPFMetrics struct {
    EventsReceived   atomic.Int64
    EventsDropped    atomic.Int64
    RingbufOverflows atomic.Int64
    PerfOverflows    atomic.Int64
}
```

---

## 7. 测试矩阵

| 内核版本 | BTF | ringbuf | 方案 | 测试环境 |
|:---|:---|:---|:---|:---|
| 5.10+ | 支持 | 支持 | CO-RE + ringbuf | Ubuntu 20.04+ |
| 5.4 | 支持 | 不支持 | CO-RE + perf buffer | CentOS 8 |
| 4.18 | 部分 | 不支持 | 非CO-RE + perf buffer | CentOS 7 |
| < 4.18 | 不支持 | 不支持 | /proc轮询 | Ubuntu 16.04 |
