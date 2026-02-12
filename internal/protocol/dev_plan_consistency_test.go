package protocol

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestDevPlanListsAllProtocolCommandsAndEvents(t *testing.T) {
	b, err := os.ReadFile("../../docs/dev-milestone1.md")
	if err != nil {
		t.Fatalf("failed to read docs/dev-milestone1.md: %v", err)
	}
	content := string(b)

	for _, cmd := range expectedCommands() {
		needle := fmt.Sprintf("`%s`", cmd)
		if !strings.Contains(content, needle) {
			t.Fatalf("docs/dev-milestone1.md missing command marker %s", needle)
		}
	}

	for _, ev := range expectedEvents() {
		needle := fmt.Sprintf("%s", ev)
		if !strings.Contains(content, needle) {
			t.Fatalf("docs/dev-milestone1.md missing event marker %q", needle)
		}
	}
}

func TestDevPlanPhase2GateScriptsListed(t *testing.T) {
	b, err := os.ReadFile("../../docs/dev-milestone1.md")
	if err != nil {
		t.Fatalf("failed to read docs/dev-milestone1.md: %v", err)
	}
	content := string(b)

	want := []string{
		"scripts/pingpong.sh",
		"scripts/smoke.sh",
		"scripts/protocol-compat-smoke.sh",
		"scripts/tui-smoke.sh",
	}
	slices.Sort(want)
	for _, script := range want {
		if !strings.Contains(content, script) {
			t.Fatalf("docs/dev-milestone1.md phase gate missing script %q", script)
		}
	}
}

func TestDevPlanMentionsTUIEvidenceArtifactCommand(t *testing.T) {
	b, err := os.ReadFile("../../docs/dev-milestone1.md")
	if err != nil {
		t.Fatalf("failed to read docs/dev-milestone1.md: %v", err)
	}
	content := string(b)
	for _, needle := range []string{
		"make e2e-tui-evidence",
		"artifacts/tui-evidence-*.log",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("docs/dev-milestone1.md missing tui evidence marker %q", needle)
		}
	}
}
