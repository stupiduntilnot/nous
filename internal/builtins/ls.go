package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nous/internal/core"
)

type lsEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func NewLSTool(cwd string) core.Tool {
	base := strings.TrimSpace(cwd)
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}
	return core.ToolFunc{
		ToolName: "ls",
		Run: func(_ context.Context, args map[string]any) (string, error) {
			rawPath, _ := args["path"].(string)
			rawPath = strings.TrimSpace(rawPath)
			if rawPath == "" {
				rawPath = "."
			}

			abs := rawPath
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(base, rawPath)
			}
			abs = filepath.Clean(abs)

			info, err := os.Stat(abs)
			if err != nil {
				return "", fmt.Errorf("ls_failed: %w", err)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("ls_not_directory")
			}

			items, err := os.ReadDir(abs)
			if err != nil {
				return "", fmt.Errorf("ls_failed: %w", err)
			}
			sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })

			out := make([]lsEntry, 0, len(items))
			for _, item := range items {
				itemInfo, err := item.Info()
				if err != nil {
					return "", fmt.Errorf("ls_failed: %w", err)
				}
				entryType := "file"
				if item.IsDir() {
					entryType = "dir"
				}
				out = append(out, lsEntry{
					Name: item.Name(),
					Type: entryType,
					Size: itemInfo.Size(),
				})
			}
			b, err := json.Marshal(out)
			if err != nil {
				return "", fmt.Errorf("ls_failed: %w", err)
			}
			return string(b), nil
		},
	}
}
