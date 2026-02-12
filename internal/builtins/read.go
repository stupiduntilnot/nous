package builtins

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"nous/internal/core"
)

func NewReadTool(cwd string) core.Tool {
	base := strings.TrimSpace(cwd)
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}

	return core.ToolFunc{
		ToolName: "read",
		Run: func(_ context.Context, args map[string]any) (string, error) {
			rawPath := resolveReadPathArg(args)
			if rawPath == "" {
				return "", fmt.Errorf("read_invalid_path")
			}

			offset, err := intArg(args, "offset", 0)
			if err != nil || offset < 0 {
				return "", fmt.Errorf("read_invalid_offset")
			}
			limit, err := intArg(args, "limit", -1)
			if err != nil || limit == 0 || limit < -1 {
				return "", fmt.Errorf("read_invalid_limit")
			}

			abs := rawPath
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(base, rawPath)
			}
			abs = filepath.Clean(abs)

			info, err := os.Stat(abs)
			if err != nil {
				return "", fmt.Errorf("read_failed: %w", err)
			}
			if info.IsDir() {
				return "", fmt.Errorf("read_is_directory")
			}

			b, err := os.ReadFile(abs)
			if err != nil {
				return "", fmt.Errorf("read_failed: %w", err)
			}
			if !utf8.Valid(b) {
				return "", fmt.Errorf("read_non_utf8")
			}

			lines := readLines(b)
			if offset >= len(lines) {
				return "", nil
			}
			end := len(lines)
			if limit > 0 && offset+limit < end {
				end = offset + limit
			}
			return strings.Join(lines[offset:end], "\n"), nil
		},
	}
}

func resolveReadPathArg(args map[string]any) string {
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

func normalizeReadPathValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case map[string]any:
		if p, ok := x["path"]; ok {
			return normalizeReadPathValue(p)
		}
	case []any:
		if len(x) > 0 {
			return normalizeReadPathValue(x[0])
		}
	}
	return ""
}

func DefaultTools(cwd string) []core.Tool {
	return []core.Tool{
		NewReadTool(cwd),
		NewBashTool(cwd),
		NewEditTool(cwd),
		NewWriteTool(cwd),
		NewGrepTool(cwd),
		NewLSTool(cwd),
		NewFindTool(cwd),
	}
}

func intArg(args map[string]any, key string, def int) (int, error) {
	v, ok := args[key]
	if !ok {
		return def, nil
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("invalid_int_arg")
	}
}

func readLines(b []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([]string, 0, 64)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(b) == 0 {
		return []string{}
	}
	return lines
}
