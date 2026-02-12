package core

import (
	"strings"
	"testing"
)

func TestDeterministicCompactorEstimateAndThreshold(t *testing.T) {
	c := NewDeterministicCompactor(CompactionSettings{
		KeepRecentTokens: 16,
		ThresholdTokens:  20,
	})
	msgs := []CompactionMessage{
		{ID: "m1", Role: "user", Text: strings.Repeat("a", 32)},
		{ID: "m2", Role: "assistant", Text: strings.Repeat("b", 32)},
	}
	tokens := c.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
	if !c.ShouldCompact(tokens) {
		t.Fatalf("expected token estimate to exceed threshold: tokens=%d", tokens)
	}
}

func TestDeterministicCompactorCompactReturnsSummaryAndFirstKeptEntry(t *testing.T) {
	c := NewDeterministicCompactor(CompactionSettings{
		KeepRecentTokens: 24,
		ThresholdTokens:  30,
	})
	msgs := []CompactionMessage{
		{ID: "m1", Role: "user", Text: strings.Repeat("u", 40)},
		{ID: "m2", Role: "assistant", Text: strings.Repeat("a", 40)},
		{ID: "m3", Role: "user", Text: strings.Repeat("n", 40)},
	}

	got, err := c.Compact(msgs, "focus decisions")
	if err != nil {
		t.Fatalf("compact failed: %v", err)
	}
	if got.Summary == "" {
		t.Fatalf("expected non-empty summary")
	}
	if !strings.Contains(got.Summary, "Instruction: focus decisions") {
		t.Fatalf("expected summary to include instruction, got: %q", got.Summary)
	}
	if got.FirstKeptEntryID != "m3" {
		t.Fatalf("unexpected first kept entry id: got=%q want=%q", got.FirstKeptEntryID, "m3")
	}
	if got.TokensBefore <= 0 {
		t.Fatalf("expected positive tokens_before, got %d", got.TokensBefore)
	}
}

func TestDeterministicCompactorCompactNoopWhenUnderBudget(t *testing.T) {
	c := NewDeterministicCompactor(CompactionSettings{
		KeepRecentTokens: 400,
		ThresholdTokens:  800,
	})
	msgs := []CompactionMessage{
		{ID: "m1", Role: "user", Text: "short"},
		{ID: "m2", Role: "assistant", Text: "short"},
	}
	if _, err := c.Compact(msgs, ""); err == nil {
		t.Fatalf("expected nothing_to_compact error")
	}
}
