package core

import "context"

type Tool interface {
	Name() string
	Execute(ctx context.Context, args map[string]any) (string, error)
}

type ToolFunc struct {
	ToolName string
	Run      func(ctx context.Context, args map[string]any) (string, error)
}

func (t ToolFunc) Name() string { return t.ToolName }

func (t ToolFunc) Execute(ctx context.Context, args map[string]any) (string, error) {
	return t.Run(ctx, args)
}
