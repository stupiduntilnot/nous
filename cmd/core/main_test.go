package main

import (
	"os"
	"path/filepath"
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

func TestResolveWorkDirDefaultsToGetwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	got, err := resolveWorkDir("")
	if err != nil {
		t.Fatalf("resolve workdir failed: %v", err)
	}
	if got != cwd {
		t.Fatalf("unexpected default workdir: got=%q want=%q", got, cwd)
	}
}

func TestResolveWorkDirUsesExplicitValue(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "subdir")
	if err := os.MkdirAll(input, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	got, err := resolveWorkDir("  " + input + "  ")
	if err != nil {
		t.Fatalf("resolve workdir failed: %v", err)
	}
	if got != input {
		t.Fatalf("unexpected explicit workdir: got=%q want=%q", got, input)
	}
}

func TestResolveWorkDirRejectsMissingPath(t *testing.T) {
	if _, err := resolveWorkDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("expected missing workdir to fail")
	}
}

func TestResolveWorkDirRejectsFilePath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "a.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	if _, err := resolveWorkDir(filePath); err == nil {
		t.Fatalf("expected file workdir to fail")
	}
}
