package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"baseline-agent/internal/asset"
	"baseline-agent/internal/client"
	"baseline-agent/internal/config"
	"baseline-agent/internal/executor"
)

func main() {
	fmt.Println("Baseline Agent v2.2.0 starting...")

	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Config loaded: ServerAddr=%s, HostID=%s\n", cfg.ServerAddr, cfg.HostID)

	// 采集资产信息
	assetInfo, err := asset.Collect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to collect asset info: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Asset info: IP=%s, Hostname=%s, OS=%s\n", assetInfo.IPAddress, assetInfo.Hostname, assetInfo.OSType)

	// 创建执行器（最大并发 2 个）
	exec := executor.NewExecutor(2)
	fmt.Println("Executor created with max concurrency 2")

	// 创建客户端
	client := client.NewClient(cfg, exec)

	// 启动客户端
	go func() {
		if err := client.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Client error: %v\n", err)
		}
	}()
	fmt.Println("Client started")

	// 等待关闭信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("Shutting down...")
	client.Close()
	fmt.Println("Agent stopped")
}
