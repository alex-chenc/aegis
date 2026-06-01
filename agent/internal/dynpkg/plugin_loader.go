package dynpkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"aegis-agent/internal/ebpf/plugin"
)

func (m *Manager) loadPlugin(ctx context.Context, pkg *InstalledPackage, extractDir string) error {
	pkg.ActiveArtifact = "ringbuf"

	pluginInfo := &plugin.PackageInfo{
		PackageID:      pkg.PackageID,
		ActiveArtifact: pkg.ActiveArtifact,
		Manifest:       convertManifest(pkg.PluginManifest),
	}

	err := plugin.LoadPlugin(pluginInfo, extractDir, func(pkgID string, event map[string]interface{}) {
		m.ProcessEvent(pkgID, event)
	})
	if err != nil {
		return fmt.Errorf("load plugin: %w", err)
	}

	for _, hook := range pkg.PluginManifest.Hooks {
		pkg.LoadedHooks = append(pkg.LoadedHooks, hook.Attach)
	}
	return nil
}

func (m *Manager) unloadPlugin(pkg *InstalledPackage) error {
	return plugin.UnloadPlugin(pkg.PackageID)
}

func (m *Manager) downloadPackage(ctx context.Context, packageURL, signatureURL string) (string, string, error) {
	tmpDir := filepath.Join(m.storagePath, "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", "", fmt.Errorf("create tmp dir: %w", err)
	}

	packagePath := filepath.Join(tmpDir, "package.tar.gz")
	if err := downloadFile(ctx, packageURL, packagePath); err != nil {
		return "", "", fmt.Errorf("download package: %w", err)
	}

	sigPath := filepath.Join(tmpDir, "package.tar.gz.sig")
	if err := downloadFile(ctx, signatureURL, sigPath); err != nil {
		os.Remove(packagePath)
		return "", "", fmt.Errorf("download signature: %w", err)
	}

	return packagePath, sigPath, nil
}

func convertManifest(pm *PluginManifest) *plugin.PluginManifest {
	if pm == nil {
		return nil
	}
	hooks := make([]plugin.PluginHook, len(pm.Hooks))
	for i, h := range pm.Hooks {
		hooks[i] = plugin.PluginHook{
			Name:       h.Name,
			AttachType: h.AttachType,
			Attach:     h.Attach,
			Program:    h.Program,
		}
	}
	events := make(map[string]plugin.EventDef)
	for k, v := range pm.EventSchema.Events {
		fields := make(map[string]plugin.FieldDef)
		for fk, fv := range v.Fields {
			fields[fk] = plugin.FieldDef{Name: fv.Name, Type: fv.Type}
		}
		events[k] = plugin.EventDef{Name: v.Name, Fields: fields}
	}
	return &plugin.PluginManifest{
		Hooks:       hooks,
		EventSchema: plugin.EventSchema{Events: events},
	}
}
