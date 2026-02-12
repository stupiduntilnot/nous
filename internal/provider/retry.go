package provider

import (
	"context"
	"net"
	"net/http"
	"time"
)

type retryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{
		maxAttempts: 3,
		baseDelay:   50 * time.Millisecond,
		maxDelay:    900 * time.Millisecond,
	}
}

func shouldRetryHTTPStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func shouldRetryTransportError(err error) bool {
	if err == nil {
		return false
	}
	if IsAbortedError(err) || context.Canceled == err || context.DeadlineExceeded == err {
		return false
	}
	if ne, ok := err.(net.Error); ok {
		return ne.Timeout() || ne.Temporary()
	}
	return true
}

func retryDelayForAttempt(policy retryPolicy, attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	delay := policy.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= policy.maxDelay {
			delay = policy.maxDelay
			break
		}
	}
	jitter := time.Duration((attempt*17)%37) * time.Millisecond
	delay += jitter
	if delay > policy.maxDelay {
		delay = policy.maxDelay
	}
	return delay
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
