package core

import "context"

type steerPendingChecker func() bool

type steerPendingCheckerKey struct{}

func withSteerPendingChecker(ctx context.Context, fn steerPendingChecker) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, steerPendingCheckerKey{}, fn)
}

func steerPendingCheckerFromContext(ctx context.Context) steerPendingChecker {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(steerPendingCheckerKey{}).(steerPendingChecker)
	return fn
}
