package protocol

import "testing"

func TestCommandDecodeValid(t *testing.T) {
	lines := [][]byte{
		[]byte(`{"v":"1","id":"req-1","type":"prompt","payload":{"text":"hello"}}`),
		[]byte(`{"v":"1","id":"req-2","type":"branch_session","payload":{"session_id":"sess-1"}}`),
	}
	for _, line := range lines {
		env, err := DecodeCommand(line)
		if err != nil {
			t.Fatalf("expected valid command, got error: %v", err)
		}
		if env.ID == "" || env.Type == "" {
			t.Fatalf("decoded envelope mismatch: %#v", env)
		}
	}
}

func TestCommandDecodeInvalid(t *testing.T) {
	tests := []struct {
		name string
		line []byte
	}{
		{"bad-json", []byte(`{"id":`)},
		{"missing-id", []byte(`{"v":"1","type":"ping","payload":{}}`)},
		{"missing-type", []byte(`{"v":"1","id":"x","payload":{}}`)},
		{"invalid-type", []byte(`{"v":"1","id":"x","type":"unknown","payload":{}}`)},
		{"bad-version", []byte(`{"v":"2","id":"x","type":"ping","payload":{}}`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeCommand(tc.line); err == nil {
				t.Fatalf("expected error for case %s", tc.name)
			}
		})
	}
}

func TestValidateEventType(t *testing.T) {
	if err := ValidateEventType(EvMessageUpdate); err != nil {
		t.Fatalf("expected valid event type, got error: %v", err)
	}
	if err := ValidateEventType(EventType("bad_event")); err == nil {
		t.Fatalf("expected invalid event type error")
	}
}
