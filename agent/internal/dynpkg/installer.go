package dynpkg

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExtractPackage extracts a tar.gz package to the target directory
func ExtractPackage(packagePath, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}

	f, err := os.Open(packagePath)
	if err != nil {
		return fmt.Errorf("open package: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Clean the entry name to strip leading "./" and resolve "." / ".."
		name := filepath.Clean(header.Name)
		if name == "." || name == "" {
			// Root directory entry (e.g. "./") — skip it, targetDir already exists
			continue
		}

		target := filepath.Join(targetDir, name)
		if !strings.HasPrefix(target, filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// ParseManifests parses package.yaml and plugin.yaml from the extracted directory
func ParseManifests(extractDir string) (*PackageManifest, *PluginManifest, error) {
	pkgPath := filepath.Join(extractDir, "package.yaml")
	pkgData, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read package.yaml: %w", err)
	}

	var pkgManifest PackageManifest
	if err := yaml.Unmarshal(pkgData, &pkgManifest); err != nil {
		return nil, nil, fmt.Errorf("parse package.yaml: %w", err)
	}

	// If plugin.manifest is set, read the external plugin.yaml file
	if pkgManifest.Plugin.Manifest != "" {
		pluginPath := filepath.Join(extractDir, pkgManifest.Plugin.Manifest)
		pluginData, err := os.ReadFile(pluginPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read plugin.yaml: %w", err)
		}

		var pluginManifest PluginManifest
		if err := yaml.Unmarshal(pluginData, &pluginManifest); err != nil {
			return nil, nil, fmt.Errorf("parse plugin.yaml: %w", err)
		}

		return &pkgManifest, &pluginManifest, nil
	}

	// Otherwise, build a PluginManifest from the inline hooks/event_schema in package.yaml
	pluginManifest := &PluginManifest{
		SchemaVersion: pkgManifest.SchemaVersion,
		PluginID:      pkgManifest.PackageID,
		PackageID:     pkgManifest.PackageID,
		Hooks:         pkgManifest.Hooks,
		EventSchema:   pkgManifest.EventSchema,
	}

	return &pkgManifest, pluginManifest, nil
}

func downloadFile(ctx context.Context, url, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") {
		data, err := os.ReadFile(url)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}
