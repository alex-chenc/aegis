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
}

// NewAssetCollector 创建资产采集器
func NewAssetCollector(logger *zap.Logger) *AssetCollector {
	return &AssetCollector{
		logger:           logger,
		packageCollector: NewPackageCollector(logger),
		processCollector: NewProcessCollector(logger, 2000),
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

	duration := time.Since(startTime)
	c.logger.Info("Asset collection completed",
		zap.Int("packages", len(snapshot.Packages)),
		zap.Int("processes", len(snapshot.Processes)),
		zap.Int("errors", len(snapshot.Errors)),
		zap.Duration("duration", duration))

	return snapshot, nil
}
