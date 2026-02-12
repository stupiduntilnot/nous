package core

import "context"

type Tool interface {
	Name() string
	Execute(ctx context.Context, args map[string]any) (string, error)
}

type ToolProgressFunc func(delta string)

type ProgressiveTool interface {
	Tool
	ExecuteWithProgress(ctx context.Context, args map[string]any, progress ToolProgressFunc) (string, error)
}

type ToolFunc struct {
	ToolName string
	Run      func(ctx context.Context, args map[string]any) (string, error)
}

func (t ToolFunc) Name() string { return t.ToolName }

func (t ToolFunc) Execute(ctx context.Context, args map[string]any) (string, error) {
	return t.Run(ctx, args)
}

type ProgressiveToolFunc struct {
	ToolName string
	Run      func(ctx context.Context, args map[string]any, progress ToolProgressFunc) (string, error)
}

func (t ProgressiveToolFunc) Name() string { return t.ToolName }

func (t ProgressiveToolFunc) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.Run == nil {
		return "", nil
	}
	return t.Run(ctx, args, nil)
}

func (t ProgressiveToolFunc) ExecuteWithProgress(ctx context.Context, args map[string]any, progress ToolProgressFunc) (string, error) {
	if t.Run == nil {
		return "", nil
	}
	return t.Run(ctx, args, progress)
}
