package builtins

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"nous/internal/core"
)

func NewGrepTool(cwd string) core.Tool {
	base := resolveBaseDir(cwd)

	return core.ToolFunc{
		ToolName: "grep",
		Run: func(_ context.Context, args map[string]any) (string, error) {
			pattern, _ := args["pattern"].(string)
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				return "", fmt.Errorf("grep_invalid_pattern")
			}

			root, _ := args["path"].(string)
			root = strings.TrimSpace(root)
			if root == "" {
				root = "."
			}
			absRoot := resolveToolPath(base, root)

			info, err := os.Stat(absRoot)
			if err != nil {
				return "", fmt.Errorf("grep_failed: %w", err)
			}

			ignoreCase, _ := args["ignore_case"].(bool)
			limit, err := intArg(args, "limit", 100)
			if err != nil || limit <= 0 {
				return "", fmt.Errorf("grep_invalid_limit")
			}

			re, err := compileGrepPattern(pattern, ignoreCase)
			if err != nil {
				return "", fmt.Errorf("grep_invalid_pattern")
			}

			matches := make([]string, 0, 32)
			appendFromFile := func(path, rel string) error {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if !utf8.Valid(b) {
					return nil
				}
				sc := bufio.NewScanner(bytes.NewReader(b))
				sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
				lineNo := 0
				for sc.Scan() {
					lineNo++
					line := sc.Text()
					if re.MatchString(line) {
						matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, lineNo, line))
						if len(matches) >= limit {
							return errLimitReached
						}
					}
				}
				return sc.Err()
			}

			if !info.IsDir() {
				rel := filepath.Base(absRoot)
				if err := appendFromFile(absRoot, rel); err != nil && err != errLimitReached {
					return "", fmt.Errorf("grep_failed: %w", err)
				}
				return strings.Join(matches, "\n"), nil
			}

			walkErr := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, inErr error) error {
				if inErr != nil {
					return inErr
				}
				if d.IsDir() {
					if d.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				rel, err := filepath.Rel(absRoot, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if err := appendFromFile(path, rel); err != nil {
					if err == errLimitReached {
						return filepath.SkipAll
					}
					return err
				}
				return nil
			})
			if walkErr != nil && walkErr != filepath.SkipAll {
				return "", fmt.Errorf("grep_failed: %w", walkErr)
			}
			return strings.Join(matches, "\n"), nil
		},
	}
}

var errLimitReached = fmt.Errorf("grep_limit_reached")

func compileGrepPattern(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	if ignoreCase {
		return regexp.Compile("(?i)" + pattern)
	}
	return regexp.Compile(pattern)
}
