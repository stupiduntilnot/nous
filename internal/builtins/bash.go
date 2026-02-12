package builtins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nous/internal/core"
)

const (
	bashOutputLimit = 50 * 1024
	bashLineLimit   = 2000
)

func NewBashTool(cwd string) core.Tool {
	base := strings.TrimSpace(cwd)
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}

	return core.ToolFunc{
		ToolName: "bash",
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			cmdText := resolveStringArgLocal(args, "command", "cmd")
			if cmdText == "" {
				return "", fmt.Errorf("bash_invalid_command")
			}

			timeoutSecs, err := floatArg(args, "timeout", 0)
			if err != nil || timeoutSecs < 0 {
				return "", fmt.Errorf("bash_invalid_timeout")
			}

			runCtx := ctx
			cancel := func() {}
			if timeoutSecs > 0 {
				runCtx, cancel = context.WithTimeout(ctx, time.Duration(float64(time.Second)*timeoutSecs))
			}
			defer cancel()

			cmd := exec.CommandContext(runCtx, "/bin/zsh", "-lc", cmdText)
			cmd.Dir = base
			raw, execErr := cmd.CombinedOutput()
			full := sanitizeBashOutput(string(raw))
			rendered, details := truncateBashOutput(full)
			if strings.TrimSpace(rendered) == "" {
				rendered = "(no output)"
			}
			if details.note != "" {
				rendered += "\n\n" + details.note
			}

			if runCtx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("%s\n\nCommand timed out after %s seconds", strings.TrimSpace(rendered), formatSeconds(timeoutSecs))
			}
			if execErr != nil {
				exitCode := exitCodeOf(execErr)
				msg := strings.TrimSpace(rendered)
				if msg == "" {
					msg = "(no output)"
				}
				if exitCode >= 0 {
					return "", fmt.Errorf("%s\n\nCommand exited with code %d", msg, exitCode)
				}
				return "", fmt.Errorf("%s", msg)
			}
			return strings.TrimSpace(rendered), nil
		},
	}
}

func resolveStringArgLocal(args map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func floatArg(args map[string]any, key string, def float64) (float64, error) {
	v, ok := args[key]
	if !ok {
		return def, nil
	}
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("invalid_float_arg")
	}
}

type bashTruncationDetails struct {
	note string
}

func truncateBashOutput(full string) (rendered string, details bashTruncationDetails) {
	if full == "" {
		return "", details
	}
	totalLines := strings.Count(full, "\n") + 1
	lines := strings.Split(full, "\n")

	truncatedBy := ""
	if len(lines) > bashLineLimit {
		lines = lines[len(lines)-bashLineLimit:]
		truncatedBy = "lines"
	}
	rendered = strings.Join(lines, "\n")

	if len(rendered) > bashOutputLimit {
		rendered = tailUTF8(rendered, bashOutputLimit)
		truncatedBy = "bytes"
	}
	rendered = strings.TrimSpace(rendered)
	if truncatedBy == "" {
		return rendered, details
	}

	path, err := writeFullBashOutput(full)
	if err != nil {
		return rendered, details
	}

	outputLines := 0
	if rendered != "" {
		outputLines = strings.Count(rendered, "\n") + 1
	}
	startLine := totalLines - outputLines + 1
	if startLine < 1 {
		startLine = 1
	}
	endLine := totalLines

	if truncatedBy == "lines" {
		details.note = fmt.Sprintf("[Showing lines %d-%d of %d. Full output: %s]", startLine, endLine, totalLines, path)
		return rendered, details
	}
	details.note = fmt.Sprintf("[Showing lines %d-%d of %d (%s limit). Full output: %s]", startLine, endLine, totalLines, formatSize(bashOutputLimit), path)
	return rendered, details
}

func writeFullBashOutput(full string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	name := "pi-bash-" + hex.EncodeToString(b) + ".log"
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, []byte(full), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func exitCodeOf(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ProcessState != nil {
		return ee.ProcessState.ExitCode()
	}
	return -1
}

func sanitizeBashOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func tailUTF8(s string, maxBytes int) string {
	b := []byte(s)
	if len(b) <= maxBytes {
		return s
	}
	start := len(b) - maxBytes
	for start < len(b) && (b[start]&0xc0) == 0x80 {
		start++
	}
	return string(b[start:])
}

func formatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
}

func formatSeconds(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}
