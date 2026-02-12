package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"nous/internal/core"
)

func NewWriteTool(cwd string) core.Tool {
	base := resolveBaseDir(cwd)

	return core.ToolFunc{
		ToolName: "write",
		Run: func(_ context.Context, args map[string]any) (string, error) {
			path := resolveWritePathArg(args)
			if path == "" {
				return "", fmt.Errorf("write_invalid_path")
			}
			content, ok := args["content"].(string)
			if !ok {
				return "", fmt.Errorf("write_invalid_content")
			}

			abs := resolveToolPath(base, path)
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return "", fmt.Errorf("write_failed: %w", err)
			}
			if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
				return "", fmt.Errorf("write_failed: %w", err)
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
		},
	}
}

func resolveWritePathArg(args map[string]any) string {
	keys := []string{"path", "file_path", "filePath", "filepath", "file", "target_path", "targetPath"}
	for _, key := range keys {
		v, ok := args[key]
		if !ok {
			continue
		}
		if s := normalizeReadPathValue(v); s != "" {
			return s
		}
	}
	return ""
}
