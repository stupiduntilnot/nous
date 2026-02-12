package protocol

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandExamplesCoveredByCommandRequirements(t *testing.T) {
	reqs := loadCommandRequirementsForTest(t)

	f, err := os.Open(filepath.FromSlash("../../docs/example-protocol-commands.ndjson"))
	if err != nil {
		t.Fatalf("open commands fixture failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		env, err := DecodeCommand(scanner.Bytes())
		if err != nil {
			t.Fatalf("invalid command fixture line %d: %v", line, err)
		}
		if _, ok := reqs[env.Type]; !ok {
			t.Fatalf("command fixture line %d has no requirement key %q", line, env.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan commands fixture failed: %v", err)
	}
}

func TestCommandRequirementsHaveExamples(t *testing.T) {
	reqs := loadCommandRequirementsForTest(t)
	seen := map[string]struct{}{}

	f, err := os.Open(filepath.FromSlash("../../docs/example-protocol-commands.ndjson"))
	if err != nil {
		t.Fatalf("open commands fixture failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		env, err := DecodeCommand(scanner.Bytes())
		if err != nil {
			t.Fatalf("invalid command fixture line: %v", err)
		}
		seen[env.Type] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan commands fixture failed: %v", err)
	}

	for key := range reqs {
		if _, ok := seen[key]; !ok {
			t.Fatalf("command requirement key %q has no fixture example", key)
		}
	}
}

func loadCommandRequirementsForTest(t *testing.T) map[string]struct{} {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash("../../docs/protocol-openapi-like.json"))
	if err != nil {
		t.Fatalf("read protocol spec failed: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode protocol spec failed: %v", err)
	}
	raw, ok := doc["x-command-payload-requirements"].(map[string]any)
	if !ok {
		t.Fatalf("x-command-payload-requirements missing or invalid")
	}
	out := make(map[string]struct{}, len(raw))
	for k := range raw {
		out[k] = struct{}{}
	}
	return out
}
