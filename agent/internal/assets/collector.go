package assets

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// AssetCollector 主机资产采集器
type AssetCollector struct {
	logger           *zap.Logger
	packageCollector *PackageCollector
	processCollector *ProcessCollector
	llmCollector     *LLMServiceCollector
	agentCollector   *AIAgentCollector
	mcpCollector     *MCPCollector
}

// NewAssetCollector 创建资产采集器
func NewAssetCollector(logger *zap.Logger) *AssetCollector {
	return &AssetCollector{
		logger:           logger,
		packageCollector: NewPackageCollector(logger),
		processCollector: NewProcessCollector(logger, 2000),
		llmCollector:     NewLLMServiceCollector(logger),
		agentCollector:   NewAIAgentCollector(logger),
		mcpCollector:     NewMCPCollector(logger),
	}
}

// Collect 执行资产采集
func (c *AssetCollector) Collect(ctx context.Context, hostID, hostname, ipAddress, osType, osVersion, arch string, opts CollectOptions) (*HostAssetSnapshot, error) {
	startTime := time.Now()
	c.logger.Info("Starting asset collection",
		zap.String("host_id", hostID),
		zap.String("hostname", hostname),
		zap.Any("options", opts))

	snapshot := &HostAssetSnapshot{
		HostID:      hostID,
		Hostname:    hostname,
		IPAddress:   ipAddress,
		OSType:      osType,
		OSVersion:   osVersion,
		Arch:        arch,
		CollectedAt: startTime,
	}

	// 采集软件包（总是执行）
	c.logger.Info("Collecting packages")
	packages, pkgErrors := c.packageCollector.DetectAndCollect(ctx, opts.IncludePackageFiles)
	snapshot.Packages = packages
	snapshot.Errors = append(snapshot.Errors, pkgErrors...)
	c.logger.Info("Package collection completed", zap.Int("count", len(packages)))

	// 采集进程
	c.logger.Info("Collecting processes")
	processes, err := c.processCollector.Collect(ctx, opts.IncludeListenPorts)
	if err != nil {
		c.logger.Error("Process collection failed", zap.Error(err))
		snapshot.Errors = append(snapshot.Errors, CollectError{
			Stage:   "process",
			Message: err.Error(),
		})
	} else {
		snapshot.Processes = processes
		c.logger.Info("Process collection completed", zap.Int("count", len(processes)))
	}

	// 采集 AI 资产（LLM 服务 / AI Agent / MCP Server）
	c.logger.Info("Collecting AI assets")
	aiAssets := c.collectAIAssets(ctx, snapshot.Processes)
	snapshot.AIAssets = aiAssets
	c.logger.Info("AI asset collection completed", zap.Int("count", len(aiAssets)))

	duration := time.Since(startTime)
	c.logger.Info("Asset collection completed",
		zap.Int("packages", len(snapshot.Packages)),
		zap.Int("processes", len(snapshot.Processes)),
		zap.Int("errors", len(snapshot.Errors)),
		zap.Duration("duration", duration))

	return snapshot, nil
}

// collectAIAssets 编排 3 个 AI 资产采集器
func (c *AssetCollector) collectAIAssets(ctx context.Context, processes []ProcessAsset) []AIAsset {
	var allAssets []AIAsset

	// 1. 构建端口 -> PID 映射（用于 LLM 服务探测）
	listenPorts := make(map[int][]int)
	for _, p := range processes {
		for _, port := range p.ListenPorts {
			if port > 0 {
				listenPorts[port] = append(listenPorts[port], p.PID)
			}
		}
	}

	// 2. LLM 服务探测
	llmAssets := c.llmCollector.Collect(ctx, listenPorts)
	allAssets = append(allAssets, llmAssets...)

	// 3. AI Agent 配置扫描
	agentAssets := c.agentCollector.Collect(ctx)
	allAssets = append(allAssets, agentAssets...)

	// 4. MCP Server 配置解析
	mcpAssets := c.mcpCollector.Collect(ctx)
	allAssets = append(allAssets, mcpAssets...)

	return allAssets
}
