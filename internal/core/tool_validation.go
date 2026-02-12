package core

import (
	"fmt"
	"strconv"
	"strings"
)

func normalizeToolArguments(toolName string, args map[string]any) (map[string]any, error) {
	switch toolName {
	case "read":
		return normalizeReadArgs(args)
	case "ls":
		return normalizeLSArgs(args)
	case "find":
		return normalizeFindArgs(args)
	case "grep":
		return normalizeGrepArgs(args)
	case "write":
		return normalizeWriteArgs(args)
	case "edit":
		return normalizeEditArgs(args)
	default:
		return args, nil
	}
}

func normalizeReadArgs(args map[string]any) (map[string]any, error) {
	path := resolveStringArg(args,
		"path", "file_path", "filePath", "filepath", "file", "target_path", "targetPath")
	if path == "" {
		return nil, fmt.Errorf("validation_failed: read.path is required")
	}
	out := map[string]any{"path": path}

	if n, ok, err := resolveIntArg(args, []string{"offset", "start_line", "startLine"}); err != nil {
		return nil, fmt.Errorf("validation_failed: read.offset must be a number")
	} else if ok {
		if n < 0 {
			return nil, fmt.Errorf("validation_failed: read.offset must be >= 0")
		}
		out["offset"] = n
	}
	if n, ok, err := resolveIntArg(args, []string{"limit", "max_lines", "maxLines"}); err != nil {
		return nil, fmt.Errorf("validation_failed: read.limit must be a number")
	} else if ok {
		if n == 0 || n < -1 {
			return nil, fmt.Errorf("validation_failed: read.limit must be > 0 or -1")
		}
		out["limit"] = n
	}
	return out, nil
}

func normalizeLSArgs(args map[string]any) (map[string]any, error) {
	path := resolveStringArg(args, "path", "dir", "directory", "target_path", "targetPath")
	if path == "" {
		path = "."
	}
	return map[string]any{"path": path}, nil
}

func normalizeFindArgs(args map[string]any) (map[string]any, error) {
	query := resolveStringArg(args, "query", "pattern")
	if query == "" {
		return nil, fmt.Errorf("validation_failed: find.query is required")
	}
	out := map[string]any{"query": query}
	path := resolveStringArg(args, "path", "dir", "directory", "target_path", "targetPath")
	if path == "" {
		path = "."
	}
	out["path"] = path

	if n, ok, err := resolveIntArg(args, []string{"max_results", "maxResults"}); err != nil {
		return nil, fmt.Errorf("validation_failed: find.max_results must be a number")
	} else if ok {
		if n <= 0 {
			return nil, fmt.Errorf("validation_failed: find.max_results must be > 0")
		}
		out["max_results"] = n
	}
	if n, ok, err := resolveIntArg(args, []string{"max_depth", "maxDepth"}); err != nil {
		return nil, fmt.Errorf("validation_failed: find.max_depth must be a number")
	} else if ok {
		if n < -1 {
			return nil, fmt.Errorf("validation_failed: find.max_depth must be >= -1")
		}
		out["max_depth"] = n
	}
	return out, nil
}

func normalizeGrepArgs(args map[string]any) (map[string]any, error) {
	pattern := resolveStringArg(args, "pattern", "query")
	if pattern == "" {
		return nil, fmt.Errorf("validation_failed: grep.pattern is required")
	}
	out := map[string]any{"pattern": pattern}
	path := resolveStringArg(args, "path", "dir", "directory", "target_path", "targetPath")
	if path == "" {
		path = "."
	}
	out["path"] = path

	if n, ok, err := resolveIntArg(args, []string{"limit", "max_results", "maxResults"}); err != nil {
		return nil, fmt.Errorf("validation_failed: grep.limit must be a number")
	} else if ok {
		if n <= 0 {
			return nil, fmt.Errorf("validation_failed: grep.limit must be > 0")
		}
		out["limit"] = n
	}
	if b, ok, err := resolveBoolArg(args, []string{"ignore_case", "ignoreCase"}); err != nil {
		return nil, fmt.Errorf("validation_failed: grep.ignore_case must be a boolean")
	} else if ok {
		out["ignore_case"] = b
	}
	return out, nil
}

func normalizeWriteArgs(args map[string]any) (map[string]any, error) {
	path := resolveStringArg(args,
		"path", "file_path", "filePath", "filepath", "file", "target_path", "targetPath")
	if path == "" {
		return nil, fmt.Errorf("validation_failed: write.path is required")
	}
	content, ok := resolveRequiredStringField(args, "content", "text", "body")
	if !ok {
		return nil, fmt.Errorf("validation_failed: write.content is required")
	}
	return map[string]any{
		"path":    path,
		"content": content,
	}, nil
}

func normalizeEditArgs(args map[string]any) (map[string]any, error) {
	path := resolveStringArg(args,
		"path", "file_path", "filePath", "filepath", "file", "target_path", "targetPath")
	if path == "" {
		return nil, fmt.Errorf("validation_failed: edit.path is required")
	}
	oldText, ok := resolveRequiredStringField(args, "oldText", "old_text")
	if !ok {
		return nil, fmt.Errorf("validation_failed: edit.oldText is required")
	}
	newText, ok := resolveRequiredStringField(args, "newText", "new_text")
	if !ok {
		return nil, fmt.Errorf("validation_failed: edit.newText is required")
	}
	return map[string]any{
		"path":    path,
		"oldText": oldText,
		"newText": newText,
	}, nil
}

func resolveRequiredStringField(args map[string]any, keys ...string) (string, bool) {
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

func resolveStringArg(args map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		if s := normalizeStringArgValue(v); s != "" {
			return s
		}
	}
	return ""
}

func normalizeStringArgValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case map[string]any:
		if p, ok := x["path"]; ok {
			return normalizeStringArgValue(p)
		}
		if p, ok := x["value"]; ok {
			return normalizeStringArgValue(p)
		}
	case []any:
		if len(x) > 0 {
			return normalizeStringArgValue(x[0])
		}
	}
	return ""
}

func resolveIntArg(args map[string]any, keys []string) (int, bool, error) {
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		n, err := toInt(v)
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}
	return 0, false, nil
}

func resolveBoolArg(args map[string]any, keys []string) (bool, bool, error) {
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		b, err := toBool(v)
		if err != nil {
			return false, true, err
		}
		return b, true, nil
	}
	return false, false, nil
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case float32:
		return int(n), nil
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, fmt.Errorf("empty_number")
		}
		if i, err := strconv.Atoi(s); err == nil {
			return i, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}
		return int(f), nil
	default:
		return 0, fmt.Errorf("not_number")
	}
}

func toBool(v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		s := strings.TrimSpace(strings.ToLower(b))
		if s == "true" {
			return true, nil
		}
		if s == "false" {
			return false, nil
		}
		return false, fmt.Errorf("not_bool")
	default:
		return false, fmt.Errorf("not_bool")
	}
}
