package builtins

import (
	"context"
	"fmt"
	"os"
	"strings"

	"nous/internal/core"
)

func NewEditTool(cwd string) core.Tool {
	base := resolveBaseDir(cwd)

	return core.ToolFunc{
		ToolName: "edit",
		Run: func(_ context.Context, args map[string]any) (string, error) {
			path := resolveWritePathArg(args)
			if path == "" {
				return "", fmt.Errorf("edit_invalid_path")
			}
			oldText, ok := resolveRequiredStringFieldLocal(args, "oldText", "old_text")
			if !ok {
				return "", fmt.Errorf("edit_invalid_old_text")
			}
			newText, ok := resolveRequiredStringFieldLocal(args, "newText", "new_text")
			if !ok {
				return "", fmt.Errorf("edit_invalid_new_text")
			}

			abs := resolveToolPath(base, path)

			b, err := os.ReadFile(abs)
			if err != nil {
				return "", fmt.Errorf("edit_failed: %w", err)
			}
			content := string(b)
			count := strings.Count(content, oldText)
			if count == 0 {
				return "", fmt.Errorf("edit_old_text_not_found")
			}
			if count > 1 {
				return "", fmt.Errorf("edit_old_text_not_unique")
			}

			updated := strings.Replace(content, oldText, newText, 1)
			if updated == content {
				return "", fmt.Errorf("edit_noop")
			}
			if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
				return "", fmt.Errorf("edit_failed: %w", err)
			}
			return fmt.Sprintf("edited %s", path), nil
		},
	}
}

func resolveRequiredStringFieldLocal(args map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return "", false
		}
		return s, true
	}
	return "", false
}
