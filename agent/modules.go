package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "ai-benchmark/agent/proto/agent_comm"
)

const (
	maxConcurrentExecs = 2
	maxOutputSize      = 1 * 1024 * 1024
)

var (
	execSemaphore = make(chan struct{}, maxConcurrentExecs)
)

func runHeartbeat(ctx context.Context, stream pb.AgentService_RegisterClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sendHeartbeat(stream)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendHeartbeat(stream)
		}
	}
}

func sendHeartbeat(stream pb.AgentService_RegisterClient) {
	v, _ := mem.VirtualMemory()

	cpuLoad := float32(0.0)
	if loadAvg, err := load.Avg(); err == nil {
		cpuLoad = float32(loadAvg.Load1)
	} else {
		c, _ := cpu.Percent(0, false)
		if len(c) > 0 {
			cpuLoad = float32(c[0])
		}
	}

	req := &pb.AgentMessage{
		HostId:    hostID,
		MessageId: fmt.Sprintf("hb-%d", time.Now().UnixNano()),
		Timestamp: timestamppb.Now(),
		Payload: &pb.AgentMessage_HeartbeatRequest{
			HeartbeatRequest: &pb.HeartbeatRequest{
				AgentVersion:    version,
				CpuLoad_1Min:    cpuLoad,
				MemUsagePercent: float32(v.UsedPercent),
			},
		},
	}
	if err := stream.Send(req); err != nil {
		log.Printf("Failed to send heartbeat: %v", err)
	} else {
		log.Printf("Heartbeat sent: cpu=%.1f%%, mem=%.1f%%", cpuLoad, v.UsedPercent)
	}
}

func runAssetCollection(ctx context.Context, stream pb.AgentService_RegisterClient) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	sendAssetInfo(stream)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendAssetInfo(stream)
		}
	}
}

func sendAssetInfo(stream pb.AgentService_RegisterClient) {
	hostInfo, _ := host.Info()
	v, _ := mem.VirtualMemory()
	c, _ := cpu.Info()

	var cpuModel string
	var cores, threads int32
	if len(c) > 0 {
		cpuModel = c[0].ModelName
		cores = int32(c[0].Cores)
		threads = cores
	}

	var totalDiskGB int64
	partitions, _ := disk.Partitions(false)
	for _, p := range partitions {
		if usage, err := disk.Usage(p.Mountpoint); err == nil {
			totalDiskGB += int64(usage.Total / (1024 * 1024 * 1024))
		}
	}

	ipAddress := getPrimaryIPAddress()

	assetInfo := &pb.AssetInfo{
		Hostname:      hostInfo.Hostname,
		IpAddress:     ipAddress,
		OsName:        getOSName(),
		OsVersion:     hostInfo.PlatformVersion,
		KernelVersion: hostInfo.KernelVersion,
		Arch:          runtime.GOARCH,
		CpuInfo: &pb.CPUInfo{
			ModelName:    cpuModel,
			Cores:        cores,
			Threads:      threads,
			FrequencyMhz: 0,
		},
		MemoryInfo: &pb.MemoryInfo{
			TotalBytes: int64(v.Total),
			FreeBytes:  int64(v.Free),
			UsedBytes:  int64(v.Used),
		},
		CollectedAt: timestamppb.Now(),
	}

	for _, p := range partitions {
		if usage, err := disk.Usage(p.Mountpoint); err == nil {
			assetInfo.DiskInfo = append(assetInfo.DiskInfo, &pb.DiskInfo{
				Device:     p.Device,
				MountPoint: p.Mountpoint,
				FsType:     p.Fstype,
				TotalBytes: int64(usage.Total),
				FreeBytes:  int64(usage.Free),
				UsedBytes:  int64(usage.Used),
			})
		}
	}

	netInterfaces, _ := net.Interfaces()
	for _, intf := range netInterfaces {
		if intf.Flags&net.FlagUp == 0 || intf.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := intf.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		netIntf := &pb.NetworkInterface{
			Name:        intf.Name,
			MacAddress:  intf.HardwareAddr.String(),
			IsUp:        true,
			IpAddresses: []string{},
		}
		for _, addr := range addrs {
			ip := addr.String()
			if idx := strings.Index(ip, "/"); idx > 0 {
				ip = ip[:idx]
			}
			if !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
				netIntf.IpAddresses = append(netIntf.IpAddresses, ip)
			}
		}
		if len(netIntf.IpAddresses) > 0 {
			assetInfo.NetworkInterfaces = append(assetInfo.NetworkInterfaces, netIntf)
		}
	}

	req := &pb.AgentMessage{
		HostId:    hostID,
		MessageId: fmt.Sprintf("asset-%d", time.Now().UnixNano()),
		Timestamp: timestamppb.Now(),
		Payload: &pb.AgentMessage_AssetInfo{
			AssetInfo: assetInfo,
		},
	}
	if err := stream.Send(req); err != nil {
		log.Printf("Failed to send asset info: %v", err)
	} else {
		log.Printf("Asset info sent: hostname=%s, ip=%s, os=%s", hostInfo.Hostname, ipAddress, hostInfo.OS)
	}
}

func getPrimaryIPAddress() string {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	for _, intf := range netInterfaces {
		if intf.Flags&net.FlagUp == 0 || intf.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := addr.String()
			if idx := strings.Index(ip, "/"); idx > 0 {
				ip = ip[:idx]
			}
			if !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
				return ip
			}
		}
	}

	return "127.0.0.1"
}

func getOSName() string {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
			}
		}
	}
	return runtime.GOOS
}

func receiveCommands(ctx context.Context, stream pb.AgentService_RegisterClient) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := stream.Recv()
		if err == io.EOF {
			log.Println("Server closed connection")
			return
		}
		if err != nil {
			log.Printf("Failed to receive message: %v", err)
			return
		}

		switch payload := msg.Payload.(type) {
		case *pb.ServerMessage_Command:
			cmd := payload.Command
			switch cmd.Type {
			case pb.CommandType_COLLECT_ASSET:
				go sendAssetInfo(stream)
			case pb.CommandType_EXEC_SCRIPT:
				go executeScript(ctx, stream, cmd)
			}
		case *pb.ServerMessage_HeartbeatResponse:
			log.Println("Heartbeat acknowledged by server")
		}
	}
}

func executeScript(ctx context.Context, stream pb.AgentService_RegisterClient, cmd *pb.ServerCommand) {
	execSemaphore <- struct{}{}
	defer func() { <-execSemaphore }()

	log.Printf("Executing script for command %s", cmd.CommandId)

	dir, err := os.MkdirTemp("", "agent_exec_*")
	if err != nil {
		sendFailedCommandResult(stream, cmd.CommandId, fmt.Sprintf("failed to create temp dir: %v", err))
		return
	}
	defer os.RemoveAll(dir)

	scriptPath := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte(cmd.ScriptContent), 0700); err != nil {
		sendFailedCommandResult(stream, cmd.CommandId, fmt.Sprintf("failed to write script: %v", err))
		return
	}

	timeout := time.Duration(cmd.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdExec := exec.CommandContext(execCtx, "/bin/bash", scriptPath)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmdExec.Stdout = &stdoutBuf
	cmdExec.Stderr = &stderrBuf

	err = cmdExec.Run()

	exitCode := 0
	status := pb.CommandStatus_SUCCESS
	errMsg := ""

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			status = pb.CommandStatus_TIMEOUT
			errMsg = "execution timeout"
		} else {
			status = pb.CommandStatus_FAILED
			errMsg = err.Error()
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if len(stdout) > maxOutputSize {
		stdout = stdout[:maxOutputSize] + "\n... (truncated)"
	}
	if len(stderr) > maxOutputSize {
		stderr = stderr[:maxOutputSize] + "\n... (truncated)"
	}

	var logEntries []*pb.LogEntry
	if stdout != "" {
		logEntries = append(logEntries, &pb.LogEntry{
			Timestamp: timestamppb.Now(),
			Stream:    pb.LogStream_STDOUT,
			Line:      stdout,
		})
	}
	if stderr != "" {
		logEntries = append(logEntries, &pb.LogEntry{
			Timestamp: timestamppb.Now(),
			Stream:    pb.LogStream_STDERR,
			Line:      stderr,
		})
	}

	res := &pb.AgentMessage{
		HostId:    hostID,
		MessageId: fmt.Sprintf("cmdres-%d", time.Now().UnixNano()),
		Timestamp: timestamppb.Now(),
		Payload: &pb.AgentMessage_CommandResult{
			CommandResult: &pb.CommandResult{
				CommandId:  cmd.CommandId,
				Status:     status,
				ExitCode:   int32(exitCode),
				Message:    errMsg,
				LogEntries: logEntries,
			},
		},
	}

	if err := stream.Send(res); err != nil {
		log.Printf("Failed to send command result: %v", err)
	} else {
		log.Printf("Command %s completed: status=%s, exit_code=%d", cmd.CommandId, status, exitCode)
	}
}

func sendFailedCommandResult(stream pb.AgentService_RegisterClient, cmdID string, errMsg string) {
	res := &pb.AgentMessage{
		HostId:    hostID,
		MessageId: fmt.Sprintf("cmdres-%d", time.Now().UnixNano()),
		Timestamp: timestamppb.Now(),
		Payload: &pb.AgentMessage_CommandResult{
			CommandResult: &pb.CommandResult{
				CommandId: cmdID,
				Status:    pb.CommandStatus_FAILED,
				Message:   errMsg,
				ExitCode:  1,
			},
		},
	}
	stream.Send(res)
}
