package core

import (
	"encoding/json"
	"fmt"
)

// AppError is the unified error envelope used across core components.
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewAppError(code, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

// MarshalJSON keeps the external shape stable and omits internals.
func (e *AppError) MarshalJSON() ([]byte, error) {
	type wire struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Cause   string `json:"cause,omitempty"`
	}
	if e == nil {
		return json.Marshal(wire{})
	}
	w := wire{Code: e.Code, Message: e.Message}
	if e.Cause != nil {
		w.Cause = e.Cause.Error()
	}
	return json.Marshal(w)
}
