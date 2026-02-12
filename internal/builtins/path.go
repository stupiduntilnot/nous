package builtins

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveBaseDir(cwd string) string {
	base := strings.TrimSpace(cwd)
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}
	base = expandPath(base)
	if base == "" {
		base = "."
	}
	if !filepath.IsAbs(base) {
		if abs, err := filepath.Abs(base); err == nil {
			base = abs
		}
	}
	return filepath.Clean(base)
}

func resolveToolPath(base, rawPath string) string {
	path := expandPath(rawPath)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}

func expandPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	path = os.Expand(path, func(name string) string {
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		return "$" + name
	})
	path = expandHome(path)
	return strings.TrimSpace(path)
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}
