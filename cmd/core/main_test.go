package main

import (
	"testing"
	"time"

	"nous/internal/extension"
)

func TestConfigureExtensionTimeouts(t *testing.T) {
	m := extension.NewManager()
	if err := configureExtensionTimeouts(m, 150*time.Millisecond, 250*time.Millisecond); err != nil {
		t.Fatalf("configure extension timeouts failed: %v", err)
	}
	if got := m.HookTimeout(); got != 150*time.Millisecond {
		t.Fatalf("unexpected hook timeout: got=%s", got)
	}
	if got := m.ToolTimeout(); got != 250*time.Millisecond {
		t.Fatalf("unexpected tool timeout: got=%s", got)
	}
}

func TestConfigureExtensionTimeoutsRejectsNegativeValues(t *testing.T) {
	m := extension.NewManager()
	if err := configureExtensionTimeouts(m, -1*time.Millisecond, 0); err == nil {
		t.Fatalf("expected negative hook timeout error")
	}
	if err := configureExtensionTimeouts(m, 0, -1*time.Millisecond); err == nil {
		t.Fatalf("expected negative tool timeout error")
	}
}
