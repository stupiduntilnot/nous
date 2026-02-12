package core

import (
	"fmt"
	"strings"
)

type CompactionMessage struct {
	ID   string
	Role string
	Text string
}

type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int
}

type CompactionSettings struct {
	KeepRecentTokens int
	ThresholdTokens  int
}

var DefaultCompactionSettings = CompactionSettings{
	KeepRecentTokens: 2000,
	ThresholdTokens:  6000,
}

type Compactor interface {
	Compact(messages []CompactionMessage, instruction string) (CompactionResult, error)
	ShouldCompact(tokens int) bool
	EstimateTokens(messages []CompactionMessage) int
}

type DeterministicCompactor struct {
	settings CompactionSettings
}

func NewDeterministicCompactor(settings CompactionSettings) *DeterministicCompactor {
	if settings.KeepRecentTokens <= 0 {
		settings.KeepRecentTokens = DefaultCompactionSettings.KeepRecentTokens
	}
	if settings.ThresholdTokens <= 0 {
		settings.ThresholdTokens = DefaultCompactionSettings.ThresholdTokens
	}
	return &DeterministicCompactor{settings: settings}
}

func (c *DeterministicCompactor) ShouldCompact(tokens int) bool {
	return tokens >= c.settings.ThresholdTokens
}

func (c *DeterministicCompactor) EstimateTokens(messages []CompactionMessage) int {
	total := 0
	for _, msg := range messages {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		// Simple deterministic estimate that is stable in tests.
		total += (len(text) + 3) / 4
		total += 4
	}
	return total
}

func (c *DeterministicCompactor) Compact(messages []CompactionMessage, instruction string) (CompactionResult, error) {
	if len(messages) == 0 {
		return CompactionResult{}, fmt.Errorf("nothing_to_compact")
	}
	tokensBefore := c.EstimateTokens(messages)
	if tokensBefore <= c.settings.KeepRecentTokens {
		return CompactionResult{}, fmt.Errorf("nothing_to_compact")
	}

	keptStart := len(messages) - 1
	keptTokens := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := c.EstimateTokens([]CompactionMessage{messages[i]})
		if keptTokens+msgTokens > c.settings.KeepRecentTokens && i != len(messages)-1 {
			break
		}
		keptTokens += msgTokens
		keptStart = i
	}
	firstKept := messages[keptStart].ID
	if strings.TrimSpace(firstKept) == "" {
		return CompactionResult{}, fmt.Errorf("missing_kept_entry_id")
	}

	summarized := messages[:keptStart]
	if len(summarized) == 0 {
		return CompactionResult{}, fmt.Errorf("nothing_to_compact")
	}
	var b strings.Builder
	b.WriteString("Compaction summary:\n")
	if strings.TrimSpace(instruction) != "" {
		b.WriteString("Instruction: ")
		b.WriteString(strings.TrimSpace(instruction))
		b.WriteByte('\n')
	}
	for _, msg := range summarized {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(msg.Role))
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteByte('\n')
	}
	summary := strings.TrimSpace(b.String())
	if summary == "" {
		return CompactionResult{}, fmt.Errorf("empty_summary")
	}
	return CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: firstKept,
		TokensBefore:     tokensBefore,
	}, nil
}
