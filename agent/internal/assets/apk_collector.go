package assets

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

// APKCollector APK 包管理器采集器
type APKCollector struct {
	logger *zap.Logger
}

// NewAPKCollector 创建 APK 采集器
func NewAPKCollector(logger *zap.Logger) *APKCollector {
	return &APKCollector{logger: logger}
}

// Collect 采集 APK 软件包
func (c *APKCollector) Collect(ctx context.Context, includeFiles bool) ([]PackageAsset, error) {
	installedFile := "/lib/apk/db/installed"

	file, err := os.Open(installedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open apk installed file: %w", err)
	}
	defer file.Close()

	var packages []PackageAsset
	var currentPkg *PackageAsset
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Text()

		// 空行表示一个包的结束
		if line == "" {
			if currentPkg != nil {
				packages = append(packages, *currentPkg)
				currentPkg = nil
			}
			continue
		}

		// APK 使用单字母前缀标识字段
		if len(line) < 2 {
			continue
		}

		field := string(line[0])
		value := line[1:]

		switch field {
		case "P": // Package name
			currentPkg = &PackageAsset{
				PackageManager: "apk",
				Metadata:       make(map[string]string),
			}
			currentPkg.Name = value
		case "V": // Version
			if currentPkg != nil {
				currentPkg.Version = value
			}
		case "A": // Architecture
			if currentPkg != nil {
				currentPkg.Architecture = value
			}
		case "o": // Origin (source package)
			if currentPkg != nil {
				currentPkg.SourceName = value
			}
		case "m": // Maintainer
			if currentPkg != nil {
				currentPkg.Metadata["maintainer"] = value
			}
		case "t": // Build time
			if currentPkg != nil {
				currentPkg.Metadata["build_time"] = value
			}
		case "c": // Commit hash
			if currentPkg != nil {
				currentPkg.Metadata["commit"] = value
			}
		case "D": // Dependencies
			if currentPkg != nil {
				currentPkg.Metadata["dependencies"] = value
			}
		case "R": // Replaces
			if currentPkg != nil {
				currentPkg.Metadata["replaces"] = value
			}
		case "p": // Provides
			if currentPkg != nil {
				currentPkg.Metadata["provides"] = value
			}
		case "L": // License
			if currentPkg != nil {
				currentPkg.License = value
			}
		case "F": // File paths (directory)
			if currentPkg != nil && includeFiles {
				currentPkg.InstallPaths = append(currentPkg.InstallPaths, value)
			}
		case "r": // File paths (files in directory)
			if currentPkg != nil && includeFiles {
				// 解析文件列表，格式为: dir/file1 dir/file2
				files := strings.Split(value, " ")
				for _, f := range files {
					f = strings.TrimSpace(f)
					if f != "" {
						currentPkg.InstallPaths = append(currentPkg.InstallPaths, f)
					}
				}
			}
		}
	}

	// 处理最后一个包
	if currentPkg != nil {
		packages = append(packages, *currentPkg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read apk installed file: %w", err)
	}

	// 限制文件数量
	if includeFiles {
		maxFiles := 200
		for i := range packages {
			if len(packages[i].InstallPaths) > maxFiles {
				packages[i].InstallPaths = packages[i].InstallPaths[:maxFiles]
			}
			packages[i].FileCount = len(packages[i].InstallPaths)
		}
	}

	return packages, nil
}
