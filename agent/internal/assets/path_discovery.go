package assets

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func discoverHomeDirs() []string {
	var candidates []string

	if envHomes := splitPathList(os.Getenv("AEGIS_AI_ASSET_HOME_DIRS")); len(envHomes) > 0 {
		candidates = append(candidates, envHomes...)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, home)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, listChildDirs("/Users")...)
	} else {
		candidates = append(candidates, "/root")
		candidates = append(candidates, listChildDirs("/home")...)
	}
	candidates = append(candidates, passwdHomeDirs()...)

	return uniqueExistingDirs(candidates)
}

func discoverProjectDirs() []string {
	var candidates []string

	if envDirs := splitPathList(os.Getenv("AEGIS_AI_ASSET_PROJECT_DIRS")); len(envDirs) > 0 {
		candidates = append(candidates, envDirs...)
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		candidates = append(candidates, cwd)
	}
	candidates = append(candidates, "/code/aegis")
	candidates = append(candidates, listChildDirs("/code")...)
	candidates = append(candidates, listChildDirs("/workspace")...)

	return uniqueExistingDirs(candidates)
}

// resolveCodexHome returns the Codex configuration directory for a discovered
// home. CODEX_HOME is process-scoped, so it is only applied to the current
// user's home; other discovered users retain their own ~/.codex directory.
func resolveCodexHome(homeDir string) string {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	currentHome, _ := os.UserHomeDir()
	if homeDir == currentHome && filepath.IsAbs(codexHome) {
		return filepath.Clean(codexHome)
	}
	return filepath.Join(homeDir, ".codex")
}

func splitPathList(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func listChildDirs(parent string) []string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(parent, entry.Name()))
		}
	}
	return dirs
}

func passwdHomeDirs() []string {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer file.Close()

	var dirs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) < 6 {
			continue
		}
		home := strings.TrimSpace(parts[5])
		if home == "" || home == "/" || home == "/nonexistent" {
			continue
		}
		if strings.HasPrefix(home, "/home/") || strings.HasPrefix(home, "/Users/") || home == "/root" {
			dirs = append(dirs, home)
		}
	}
	return dirs
}

func uniqueExistingDirs(candidates []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		clean := filepath.Clean(dir)
		if seen[clean] {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[clean] = true
		result = append(result, clean)
	}

	return result
}
