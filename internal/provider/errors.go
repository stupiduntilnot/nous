package provider

import (
	"errors"
	"fmt"
	"strings"
)

type AbortedError struct {
	Reason string
	Err    error
}

func (e *AbortedError) Error() string {
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "aborted"
	}
	if e.Err == nil {
		return reason
	}
	return fmt.Sprintf("%s: %v", reason, e.Err)
}

func (e *AbortedError) Unwrap() error {
	return e.Err
}

func NewAbortedError(reason string, err error) error {
	return &AbortedError{Reason: strings.TrimSpace(reason), Err: err}
}

func IsAbortedError(err error) bool {
	var target *AbortedError
	return errors.As(err, &target)
}

func AbortReason(err error) string {
	var target *AbortedError
	if errors.As(err, &target) {
		if strings.TrimSpace(target.Reason) != "" {
			return strings.TrimSpace(target.Reason)
		}
	}
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

type RetryExhaustedError struct {
	Attempts int
	LastErr  error
}

func (e *RetryExhaustedError) Error() string {
	if e == nil {
		return "retry_exhausted"
	}
	if e.LastErr == nil {
		return fmt.Sprintf("retry_exhausted after %d attempts", e.Attempts)
	}
	return fmt.Sprintf("retry_exhausted after %d attempts: %v", e.Attempts, e.LastErr)
}

func (e *RetryExhaustedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.LastErr
}

func IsRetryExhaustedError(err error) bool {
	var target *RetryExhaustedError
	return errors.As(err, &target)
}
