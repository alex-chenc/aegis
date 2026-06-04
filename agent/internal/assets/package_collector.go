package assets

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
)

// PackageCollector 包管理器采集器
type PackageCollector struct {
	logger *zap.Logger
	rpm    *RPMCollector
	dpkg   *DPKGCollector
	apk    *APKCollector
}

// NewPackageCollector 创建包管理器采集器
func NewPackageCollector(logger *zap.Logger) *PackageCollector {
	return &PackageCollector{
		logger: logger,
		rpm:    NewRPMCollector(logger),
		dpkg:   NewDPKGCollector(logger),
		apk:    NewAPKCollector(logger),
	}
}

// DetectAndCollect 检测包管理器并采集软件包
func (c *PackageCollector) DetectAndCollect(ctx context.Context, includeFiles bool) ([]PackageAsset, []CollectError) {
	var allPackages []PackageAsset
	var errors []CollectError

	// 检测并采集 RPM 包
	if c.detectRPM() {
		c.logger.Info("Detected RPM package manager")
		start := time.Now()
		packages, err := c.rpm.Collect(ctx, includeFiles)
		if err != nil {
			c.logger.Error("RPM collection failed", zap.Error(err))
			errors = append(errors, CollectError{Stage: "rpm", Message: err.Error()})
		} else {
			allPackages = append(allPackages, packages...)
			c.logger.Info("RPM collection completed",
				zap.Int("count", len(packages)),
				zap.Duration("duration", time.Since(start)))
		}
	}

	// 检测并采集 DPKG 包
	if c.detectDPKG() {
		c.logger.Info("Detected DPKG package manager")
		start := time.Now()
		packages, err := c.dpkg.Collect(ctx, includeFiles)
		if err != nil {
			c.logger.Error("DPKG collection failed", zap.Error(err))
			errors = append(errors, CollectError{Stage: "dpkg", Message: err.Error()})
		} else {
			allPackages = append(allPackages, packages...)
			c.logger.Info("DPKG collection completed",
				zap.Int("count", len(packages)),
				zap.Duration("duration", time.Since(start)))
		}
	}

	// 检测并采集 APK 包
	if c.detectAPK() {
		c.logger.Info("Detected APK package manager")
		start := time.Now()
		packages, err := c.apk.Collect(ctx, includeFiles)
		if err != nil {
			c.logger.Error("APK collection failed", zap.Error(err))
			errors = append(errors, CollectError{Stage: "apk", Message: err.Error()})
		} else {
			allPackages = append(allPackages, packages...)
			c.logger.Info("APK collection completed",
				zap.Int("count", len(packages)),
				zap.Duration("duration", time.Since(start)))
		}
	}

	if len(allPackages) == 0 && len(errors) == 0 {
		c.logger.Warn("No package manager detected")
	}

	return allPackages, errors
}

// detectRPM 检测是否为 RPM 系统
func (c *PackageCollector) detectRPM() bool {
	rpmPaths := []string{
		"/var/lib/rpm",
		"/var/lib/rpm/Packages",
		"/var/lib/rpm/rpmdb.sqlite",
	}
	for _, path := range rpmPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// detectDPKG 检测是否为 DPKG 系统
func (c *PackageCollector) detectDPKG() bool {
	_, err := os.Stat("/var/lib/dpkg/status")
	return err == nil
}

// detectAPK 检测是否为 APK 系统
func (c *PackageCollector) detectAPK() bool {
	_, err := os.Stat("/lib/apk/db/installed")
	return err == nil
}

// GenerateFingerprint 生成软件包指纹
func GenerateFingerprint(hostID, packageManager, name, version, release, architecture string) string {
	data := fmt.Sprintf("%s:%s:%s:%s:%s:%s", hostID, packageManager, name, version, release, architecture)
	return fmt.Sprintf("%x", sha256Sum(data))
}

func sha256Sum(data string) [32]byte {
	return sha256.Sum256([]byte(data))
}
