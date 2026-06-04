package assets

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ProcessCollector 进程采集器
type ProcessCollector struct {
	logger          *zap.Logger
	maxProcessCount int
	bootTime        int64          // 缓存系统启动时间
	userCache       map[int]string // UID -> username 缓存
}

// NewProcessCollector 创建进程采集器
func NewProcessCollector(logger *zap.Logger, maxProcessCount int) *ProcessCollector {
	if maxProcessCount <= 0 {
		maxProcessCount = 2000
	}
	return &ProcessCollector{
		logger:          logger,
		maxProcessCount: maxProcessCount,
		bootTime:        0,
		userCache:       make(map[int]string),
	}
}

// Collect 采集进程快照
func (c *ProcessCollector) Collect(ctx context.Context, includeListenPorts bool) ([]ProcessAsset, error) {
	processes, _, _, err := c.CollectPage(ctx, includeListenPorts, 0, c.maxProcessCount)
	return processes, err
}

// CollectPage 采集进程快照分片
func (c *ProcessCollector) CollectPage(ctx context.Context, includeListenPorts bool, offset, limit int) ([]ProcessAsset, int, bool, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > c.maxProcessCount {
		limit = c.maxProcessCount
	}

	procDir := "/proc"

	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to read /proc: %w", err)
	}

	// 预加载用户缓存和启动时间缓存
	c.loadUserCache()
	c.getBootTimeCached()

	// 预先构建 socket inode 到端口的映射
	var portMap map[string][]int
	if includeListenPorts {
		portMap = c.buildSocketPortMap()
	}

	var processes []ProcessAsset
	count := 0
	hasMore := false

	for _, entry := range entries {
		// 检查 context 取消
		select {
		case <-ctx.Done():
			c.logger.Warn("Process collection cancelled", zap.Int("collected", count))
			return processes, count, hasMore, ctx.Err()
		default:
		}

		// 只处理数字目录（PID）
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// 跳过内核线程 (PID 0)
		if pid == 0 {
			continue
		}

		// 检查是否超过最大进程数
		if count >= c.maxProcessCount {
			c.logger.Warn("Reached max process count", zap.Int("max", c.maxProcessCount))
			break
		}

		proc, err := c.collectProcess(pid, portMap)
		if err != nil {
			// 进程可能已经消失，忽略错误
			continue
		}

		// 跳过内核线程（cmdline 为空且无 exe）
		if proc.Cmdline == "" && proc.ExePath == "" {
			continue
		}

		// 脱敏 cmdline
		proc.Cmdline = RedactCmdline(proc.Cmdline)

		if count >= offset && len(processes) < limit {
			processes = append(processes, *proc)
		} else if count >= offset+limit {
			hasMore = true
			break
		}
		count++
	}

	return processes, count, hasMore, nil
}

// collectProcess 采集单个进程信息
func (c *ProcessCollector) collectProcess(pid int, portMap map[string][]int) (*ProcessAsset, error) {
	procPath := filepath.Join("/proc", strconv.Itoa(pid))

	proc := &ProcessAsset{
		PID: pid,
	}

	// 读取 comm
	if data, err := os.ReadFile(filepath.Join(procPath, "comm")); err == nil {
		proc.Comm = strings.TrimSpace(string(data))
	}

	// 读取 cmdline
	if data, err := os.ReadFile(filepath.Join(procPath, "cmdline")); err == nil {
		// cmdline 以 null 字节分隔参数
		proc.Cmdline = strings.ReplaceAll(strings.TrimRight(string(data), "\x00"), "\x00", " ")
	}

	// 读取 exe 链接
	if exe, err := os.Readlink(filepath.Join(procPath, "exe")); err == nil {
		proc.ExePath = exe
	}

	// 读取 cwd 链接
	if cwd, err := os.Readlink(filepath.Join(procPath, "cwd")); err == nil {
		proc.Cwd = cwd
	}

	// 读取 status 获取 UID 和 PPID
	c.readStatus(procPath, proc)

	// 读取 stat 获取启动时间
	c.readStat(procPath, proc)

	// 读取 cgroup 获取容器 ID
	c.readCgroup(procPath, proc)

	// 匹配监听端口
	if portMap != nil {
		proc.ListenPorts = c.matchProcessPorts(procPath, portMap)
	}

	// 获取用户名（UID 0 是 root，也需要查询）
	proc.Username = c.getUsername(proc.UID)

	return proc, nil
}

// readStatus 读取 /proc/{pid}/status
func (c *ProcessCollector) readStatus(procPath string, proc *ProcessAsset) {
	file, err := os.Open(filepath.Join(procPath, "status"))
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "PPid":
			proc.PPID, _ = strconv.Atoi(value)
		case "Uid":
			// 取第一个 UID（real uid）
			uids := strings.Fields(value)
			if len(uids) > 0 {
				proc.UID, _ = strconv.Atoi(uids[0])
			}
		}
	}
}

// readStat 读取 /proc/{pid}/stat 获取启动时间
func (c *ProcessCollector) readStat(procPath string, proc *ProcessAsset) {
	data, err := os.ReadFile(filepath.Join(procPath, "stat"))
	if err != nil {
		return
	}

	content := string(data)
	// 格式: pid (comm) state ppid pgrp session tty_nr tpgi flags minflt cminflt majflt cmajflt utime stime cutime cstime priority nice num_threads itrealvalue starttime
	// comm 可能包含括号，所以需要特殊处理
	startIdx := strings.LastIndex(content, ")")
	if startIdx < 0 {
		return
	}

	fields := strings.Fields(content[startIdx+1:])
	// proc(5) 中 starttime 是第 22 个字段；去掉 pid 和 comm 后，索引为 19。
	if len(fields) >= 20 {
		startTicks, err := strconv.ParseInt(fields[19], 10, 64)
		if err == nil && startTicks > 0 {
			// 将 ticks 转换为时间（使用缓存的启动时间）
			if bootTime := c.getBootTimeCached(); bootTime > 0 {
				clkTck := int64(100) // 通常为 100，可通过 sysconf 获取
				startSec := bootTime + startTicks/clkTck
				proc.StartTime = time.Unix(startSec, 0)
			}
		}
	}
}

// readCgroup 读取 /proc/{pid}/cgroup 获取容器 ID
func (c *ProcessCollector) readCgroup(procPath string, proc *ProcessAsset) {
	data, err := os.ReadFile(filepath.Join(procPath, "cgroup"))
	if err != nil {
		return
	}

	content := string(data)
	// 查找 Docker 容器 ID
	// 格式示例: 12:cpuset:/docker/abc123def456
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "/docker/") {
			parts := strings.Split(line, "/docker/")
			if len(parts) >= 2 {
				containerID := strings.TrimSpace(parts[1])
				if len(containerID) >= 12 {
					proc.ContainerID = containerID[:12]
					return
				}
			}
		}
	}
}

// buildSocketPortMap 构建 socket inode 到端口的映射
func (c *ProcessCollector) buildSocketPortMap() map[string][]int {
	portMap := make(map[string][]int)

	// 解析 /proc/net/tcp 和 /proc/net/tcp6
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		c.parseNetTCP(file, portMap)
	}

	return portMap
}

// parseNetTCP 解析 /proc/net/tcp 文件
func (c *ProcessCollector) parseNetTCP(file string, portMap map[string][]int) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// 跳过标题行
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		// 只处理 LISTEN 状态 (0A)
		if fields[3] != "0A" {
			continue
		}

		// 解析本地地址获取端口
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) != 2 {
			continue
		}

		port, err := strconv.ParseInt(parts[1], 16, 32)
		if err != nil {
			continue
		}

		// 获取 inode
		inode := fields[9]
		if inode != "0" {
			portMap[inode] = append(portMap[inode], int(port))
		}
	}
}

// matchProcessPorts 匹配进程的监听端口
func (c *ProcessCollector) matchProcessPorts(procPath string, portMap map[string][]int) []int {
	fdDir := filepath.Join(procPath, "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil
	}

	portSet := make(map[int]bool)
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}

		// 格式: socket:[inode]
		if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
			inode := link[8 : len(link)-1]
			if ports, ok := portMap[inode]; ok {
				for _, port := range ports {
					portSet[port] = true
				}
			}
		}
	}

	var ports []int
	for port := range portSet {
		ports = append(ports, port)
	}
	return ports
}

// getBootTimeCached 获取缓存的系统启动时间
func (c *ProcessCollector) getBootTimeCached() int64 {
	if c.bootTime > 0 {
		return c.bootTime
	}

	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				btime, _ := strconv.ParseInt(fields[1], 10, 64)
				c.bootTime = btime
				return btime
			}
		}
	}
	return 0
}

// loadUserCache 加载 /etc/passwd 到缓存
func (c *ProcessCollector) loadUserCache() {
	if len(c.userCache) > 0 {
		return
	}

	file, err := os.Open("/etc/passwd")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			if uid, err := strconv.Atoi(parts[2]); err == nil {
				c.userCache[uid] = parts[0]
			}
		}
	}
}

// getUsername 根据 UID 获取用户名（使用缓存）
func (c *ProcessCollector) getUsername(uid int) string {
	// 确保缓存已加载
	c.loadUserCache()

	if username, ok := c.userCache[uid]; ok {
		return username
	}

	return strconv.Itoa(uid)
}
