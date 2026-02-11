package protocol

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResponseExamplesCoveredByResponseRequirements(t *testing.T) {
	reqs := loadResponseRequirementsForTest(t)

	f, err := os.Open(filepath.FromSlash("../../docs/protocol/examples/responses.ndjson"))
	if err != nil {
		t.Fatalf("open responses fixture failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		var resp ResponseEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("invalid response fixture line %d: %v", line, err)
		}
		if !resp.OK {
			continue
		}

		key := resp.Type
		if resp.Type == "accepted" {
			if cmd, _ := resp.Payload["command"].(string); cmd != "" {
				key = resp.Type + ":" + cmd
			}
		}
		if _, ok := reqs[key]; !ok {
			t.Fatalf("response fixture line %d has no response requirement key %q", line, key)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses fixture failed: %v", err)
	}
}

func TestResponseRequirementsHaveSuccessExamples(t *testing.T) {
	reqs := loadResponseRequirementsForTest(t)
	seen := make(map[string]struct{}, len(reqs))

	f, err := os.Open(filepath.FromSlash("../../docs/protocol/examples/responses.ndjson"))
	if err != nil {
		t.Fatalf("open responses fixture failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var resp ResponseEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("invalid response fixture line: %v", err)
		}
		if !resp.OK {
			continue
		}
		key := resp.Type
		if resp.Type == "accepted" {
			if cmd, _ := resp.Payload["command"].(string); cmd != "" {
				key = resp.Type + ":" + cmd
			}
		}
		seen[key] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses fixture failed: %v", err)
	}

	for key := range reqs {
		if _, ok := seen[key]; !ok {
			t.Fatalf("response requirement key %q has no success fixture example", key)
		}
	}
}

func loadResponseRequirementsForTest(t *testing.T) map[string]struct{} {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash("../../docs/protocol/openapi-like.json"))
	if err != nil {
		t.Fatalf("read protocol spec failed: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode protocol spec failed: %v", err)
	}
	raw, ok := doc["x-response-payload-requirements"].(map[string]any)
	if !ok {
		t.Fatalf("x-response-payload-requirements missing or invalid")
	}
	out := make(map[string]struct{}, len(raw))
	for k := range raw {
		out[k] = struct{}{}
	}
	return out
}
