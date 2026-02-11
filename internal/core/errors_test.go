package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestErrorEnvelope(t *testing.T) {
	root := errors.New("socket closed")
	err := NewAppError("transport_error", "failed to read command", root)

	if !errors.Is(err, root) {
		t.Fatalf("expected wrapped cause to be discoverable with errors.Is")
	}

	if !strings.Contains(err.Error(), "transport_error") {
		t.Fatalf("expected error string to contain code, got: %q", err.Error())
	}

	payload, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}

	var decoded map[string]any
	if unmarshalErr := json.Unmarshal(payload, &decoded); unmarshalErr != nil {
		t.Fatalf("unmarshal failed: %v", unmarshalErr)
	}

	if decoded["code"] != "transport_error" {
		t.Fatalf("unexpected code: %#v", decoded["code"])
	}
	if decoded["message"] != "failed to read command" {
		t.Fatalf("unexpected message: %#v", decoded["message"])
	}
	if _, exists := decoded["cause"]; exists {
		t.Fatalf("cause should not be serialized")
	}
}
