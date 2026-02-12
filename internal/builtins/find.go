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

func NewFindTool(cwd string) core.Tool {
	base := resolveBaseDir(cwd)
	return core.ToolFunc{
		ToolName: "find",
		Run: func(_ context.Context, args map[string]any) (string, error) {
			query, _ := args["query"].(string)
			query = strings.TrimSpace(query)
			if query == "" {
				return "", fmt.Errorf("find_invalid_query")
			}

			root, _ := args["path"].(string)
			root = strings.TrimSpace(root)
			if root == "" {
				root = "."
			}
			absRoot := resolveToolPath(base, root)

			info, err := os.Stat(absRoot)
			if err != nil {
				return "", fmt.Errorf("find_failed: %w", err)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("find_not_directory")
			}

			maxResults, err := intArg(args, "max_results", 200)
			if err != nil || maxResults <= 0 {
				return "", fmt.Errorf("find_invalid_max_results")
			}
			maxDepth, err := intArg(args, "max_depth", -1)
			if err != nil || maxDepth < -1 {
				return "", fmt.Errorf("find_invalid_max_depth")
			}

			matches := make([]string, 0, 32)
			walkErr := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, inErr error) error {
				if inErr != nil {
					return inErr
				}
				rel, err := filepath.Rel(absRoot, path)
				if err != nil {
					return err
				}
				if rel == "." {
					return nil
				}

				if maxDepth >= 0 {
					depth := strings.Count(rel, string(os.PathSeparator)) + 1
					if depth > maxDepth {
						if d.IsDir() {
							return filepath.SkipDir
						}
						return nil
					}
				}

				if strings.Contains(rel, query) {
					matches = append(matches, rel)
					if len(matches) >= maxResults {
						return filepath.SkipAll
					}
				}
				return nil
			})
			if walkErr != nil {
				return "", fmt.Errorf("find_failed: %w", walkErr)
			}
			sort.Strings(matches)
			b, err := json.Marshal(matches)
			if err != nil {
				return "", fmt.Errorf("find_failed: %w", err)
			}
			return string(b), nil
		},
	}
}
