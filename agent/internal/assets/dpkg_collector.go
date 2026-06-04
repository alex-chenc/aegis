package assets

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// DPKGCollector DPKG 包管理器采集器
type DPKGCollector struct {
	logger *zap.Logger
}

// NewDPKGCollector 创建 DPKG 采集器
func NewDPKGCollector(logger *zap.Logger) *DPKGCollector {
	return &DPKGCollector{logger: logger}
}

// Collect 采集 DPKG 软件包
func (c *DPKGCollector) Collect(ctx context.Context, includeFiles bool) ([]PackageAsset, error) {
	statusFile := "/var/lib/dpkg/status"

	file, err := os.Open(statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open dpkg status file: %w", err)
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
				// 只采集已安装的包
				if currentPkg.Metadata["status"] == "install ok installed" {
					if includeFiles {
						files := c.collectFiles(currentPkg.Name)
						currentPkg.InstallPaths = files
						currentPkg.FileCount = len(files)
					}
					packages = append(packages, *currentPkg)
				}
				currentPkg = nil
			}
			continue
		}

		// 新的包开始
		if !strings.HasPrefix(line, " ") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "Package" {
				currentPkg = &PackageAsset{
					PackageManager: "dpkg",
					Metadata:       make(map[string]string),
				}
				currentPkg.Name = value
			} else if currentPkg != nil {
				c.setField(currentPkg, key, value)
			}
		} else if currentPkg != nil {
			// 续行
			trimmed := strings.TrimSpace(line)
			if lastKey := currentPkg.Metadata["_last_key"]; lastKey != "" {
				c.setField(currentPkg, lastKey, trimmed)
			}
		}
	}

	// 处理最后一个包
	if currentPkg != nil {
		if currentPkg.Metadata["status"] == "install ok installed" {
			if includeFiles {
				files := c.collectFiles(currentPkg.Name)
				currentPkg.InstallPaths = files
				currentPkg.FileCount = len(files)
			}
			packages = append(packages, *currentPkg)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read dpkg status: %w", err)
	}

	return packages, nil
}

// setField 设置包字段
func (c *DPKGCollector) setField(pkg *PackageAsset, key, value string) {
	switch key {
	case "Package":
		pkg.Name = value
	case "Version":
		pkg.Version = value
	case "Architecture":
		pkg.Architecture = value
	case "Source":
		pkg.SourceName = value
	case "Maintainer":
		pkg.Metadata["maintainer"] = value
	case "Installed-Size":
		pkg.Metadata["installed_size"] = value
	case "Status":
		pkg.Metadata["status"] = value
	case "Conffiles":
		pkg.Metadata["conffiles"] = value
	}
	// 记录最后一个 key 用于续行处理
	pkg.Metadata["_last_key"] = key
}

// collectFiles 采集软件包安装的文件列表
func (c *DPKGCollector) collectFiles(packageName string) []string {
	listFile := filepath.Join("/var/lib/dpkg/info", packageName+".list")

	file, err := os.Open(listFile)
	if err != nil {
		return nil
	}
	defer file.Close()

	var files []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	// 限制文件数量
	maxFiles := 200
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	return files
}

// collectConfigFiles 采集配置文件列表
func (c *DPKGCollector) collectConfigFiles(packageName string) []string {
	conffileFile := filepath.Join("/var/lib/dpkg/info", packageName+".conffiles")

	file, err := os.Open(conffileFile)
	if err != nil {
		return nil
	}
	defer file.Close()

	var files []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	return files
}
