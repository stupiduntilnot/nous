package builtins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"oh-my-agent/internal/core"
)

const bashOutputLimit = 32 * 1024

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

			timeoutSecs, err := intArg(args, "timeout", 0)
			if err != nil || timeoutSecs < 0 {
				return "", fmt.Errorf("bash_invalid_timeout")
			}

			runCtx := ctx
			cancel := func() {}
			if timeoutSecs > 0 {
				runCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
			}
			defer cancel()

			cmd := exec.CommandContext(runCtx, "/bin/zsh", "-lc", cmdText)
			cmd.Dir = base
			out, execErr := cmd.CombinedOutput()

			text := string(out)
			if len(text) > bashOutputLimit {
				text = text[len(text)-bashOutputLimit:]
				text = "[truncated]\n" + text
			}
			text = strings.TrimSpace(text)
			if text == "" {
				text = "(no output)"
			}

			if runCtx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("bash_timeout")
			}
			if execErr != nil {
				return "", fmt.Errorf("bash_failed: %s", text)
			}
			return text, nil
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
