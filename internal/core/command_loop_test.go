package core

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type gatedExecutor struct {
	started chan string
	release chan struct{}

	mu    sync.Mutex
	calls []string
}

func newGatedExecutor() *gatedExecutor {
	return &gatedExecutor{
		started: make(chan string, 64),
		release: make(chan struct{}, 64),
	}
}

func (g *gatedExecutor) Prompt(ctx context.Context, _ string, prompt string) (string, error) {
	g.mu.Lock()
	g.calls = append(g.calls, prompt)
	g.mu.Unlock()

	g.started <- prompt
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-g.release:
		return "ok:" + prompt, nil
	}
}

func (g *gatedExecutor) Calls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.calls)
}

func waitLoopIdle(t *testing.T, loop *CommandLoop) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if loop.State() == StateIdle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("loop did not return to idle")
}

type coordinatedExecutor struct {
	gated *gatedExecutor

	mu       sync.Mutex
	begins   []string
	ends     []string
	beginErr error
}

func newCoordinatedExecutor() *coordinatedExecutor {
	return &coordinatedExecutor{gated: newGatedExecutor()}
}

func (c *coordinatedExecutor) Prompt(ctx context.Context, runID, prompt string) (string, error) {
	return c.gated.Prompt(ctx, runID, prompt)
}

func (c *coordinatedExecutor) BeginRun(runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.beginErr != nil {
		return c.beginErr
	}
	c.begins = append(c.begins, runID)
	return nil
}

func (c *coordinatedExecutor) EndRun(runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ends = append(c.ends, runID)
	return nil
}

func (c *coordinatedExecutor) beginsAndEnds() ([]string, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.begins), slices.Clone(c.ends)
}

func TestCommandLoopSteerPreemptsFollowUps(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)

	if _, err := loop.Prompt("p0"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}

	if err := loop.FollowUp("f1"); err != nil {
		t.Fatalf("follow_up failed: %v", err)
	}
	if err := loop.Steer("s1"); err != nil {
		t.Fatalf("steer failed: %v", err)
	}
	if err := loop.FollowUp("f2"); err != nil {
		t.Fatalf("follow_up failed: %v", err)
	}
	if err := loop.Steer("s2"); err != nil {
		t.Fatalf("steer failed: %v", err)
	}

	for range 8 {
		exec.release <- struct{}{}
	}
	waitLoopIdle(t, loop)

	got := exec.Calls()
	want := []string{"p0", "s1", "s2", "f1", "f2"}
	if len(got) != len(want) {
		t.Fatalf("unexpected calls length: got=%d want=%d calls=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected call order at %d: got=%s want=%s all=%v", i, got[i], want[i], got)
		}
	}
}

func TestCommandLoopCoordinatesSingleRunLifecycleAcrossQueuedTurns(t *testing.T) {
	exec := newCoordinatedExecutor()
	loop := NewCommandLoop(exec)

	runID, err := loop.Prompt("p0")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.gated.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}
	if err := loop.FollowUp("f1"); err != nil {
		t.Fatalf("follow_up failed: %v", err)
	}
	exec.gated.release <- struct{}{}
	exec.gated.release <- struct{}{}

	waitLoopIdle(t, loop)
	calls := exec.gated.Calls()
	if !slices.Equal(calls, []string{"p0", "f1"}) {
		t.Fatalf("unexpected turns: %v", calls)
	}

	begins, ends := exec.beginsAndEnds()
	if len(begins) != 1 || begins[0] != runID {
		t.Fatalf("unexpected begin lifecycle calls: run_id=%q begins=%v", runID, begins)
	}
	if len(ends) != 1 || ends[0] != runID {
		t.Fatalf("unexpected end lifecycle calls: run_id=%q ends=%v", runID, ends)
	}
}

func TestCommandLoopSteerModeAllBatchesQueuedMessages(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)
	if err := loop.SetSteeringMode(QueueModeAll); err != nil {
		t.Fatalf("set steering mode failed: %v", err)
	}

	if _, err := loop.Prompt("p0"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}

	if err := loop.Steer("s1"); err != nil {
		t.Fatalf("steer s1 failed: %v", err)
	}
	if err := loop.Steer("s2"); err != nil {
		t.Fatalf("steer s2 failed: %v", err)
	}

	exec.release <- struct{}{}
	exec.release <- struct{}{}
	waitLoopIdle(t, loop)

	got := exec.Calls()
	want := []string{"p0", "s1\ns2"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected batched steer calls\nwant=%v\ngot=%v", want, got)
	}
}

func TestCommandLoopFollowUpModeAllBatchesQueuedMessages(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)
	if err := loop.SetFollowUpMode(QueueModeAll); err != nil {
		t.Fatalf("set follow-up mode failed: %v", err)
	}

	if _, err := loop.Prompt("p0"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}

	if err := loop.FollowUp("f1"); err != nil {
		t.Fatalf("follow_up f1 failed: %v", err)
	}
	if err := loop.FollowUp("f2"); err != nil {
		t.Fatalf("follow_up f2 failed: %v", err)
	}

	exec.release <- struct{}{}
	exec.release <- struct{}{}
	waitLoopIdle(t, loop)

	got := exec.Calls()
	want := []string{"p0", "f1\nf2"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected batched follow-up calls\nwant=%v\ngot=%v", want, got)
	}
}

func TestCommandLoopRejectsInvalidQueueMode(t *testing.T) {
	loop := NewCommandLoop(newGatedExecutor())
	if err := loop.SetSteeringMode(QueueMode("bad")); err == nil {
		t.Fatalf("expected invalid steering mode to fail")
	}
	if err := loop.SetFollowUpMode(QueueMode("bad")); err == nil {
		t.Fatalf("expected invalid follow-up mode to fail")
	}
}

func TestPromptWithExecutionTextUsesExecutionPayloadButPersistsInput(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)

	results := make(chan TurnResult, 1)
	loop.SetOnTurnEnd(func(r TurnResult) { results <- r })

	if _, err := loop.PromptWithExecutionText("raw-user-input", "contextualized-input"); err != nil {
		t.Fatalf("prompt with execution text failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("turn did not start")
	}
	exec.release <- struct{}{}
	waitLoopIdle(t, loop)

	gotCalls := exec.Calls()
	if !slices.Equal(gotCalls, []string{"contextualized-input"}) {
		t.Fatalf("unexpected execution prompt payloads: %v", gotCalls)
	}
	select {
	case r := <-results:
		if r.Input != "raw-user-input" {
			t.Fatalf("expected turn result input to keep raw text, got: %q", r.Input)
		}
	case <-time.After(time.Second):
		t.Fatalf("did not receive turn result")
	}
}

func TestCommandLoopAbortCancelsAndClearsQueue(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)

	if _, err := loop.Prompt("p0"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}
	if err := loop.FollowUp("f1"); err != nil {
		t.Fatalf("follow_up failed: %v", err)
	}

	if err := loop.Abort(); err != nil {
		t.Fatalf("abort failed: %v", err)
	}

	waitLoopIdle(t, loop)
	got := exec.Calls()
	if len(got) != 1 || got[0] != "p0" {
		t.Fatalf("abort should prevent queued turns: %v", got)
	}
	if err := loop.Abort(); err == nil {
		t.Fatalf("abort should fail when no active run")
	}
}

func TestCommandLoopConcurrentCommandsNoPriorityDrift(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)

	if _, err := loop.Prompt("p0"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = loop.FollowUp("f" + string(rune('a'+n)))
		}(i)
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = loop.Steer("s" + string(rune('a'+n)))
		}(i)
	}
	wg.Wait()

	for range 32 {
		exec.release <- struct{}{}
	}
	waitLoopIdle(t, loop)

	got := exec.Calls()
	if len(got) != 21 {
		t.Fatalf("unexpected number of executed turns: %d calls=%v", len(got), got)
	}

	seenFollowUp := false
	for _, call := range got[1:] {
		isSteer := len(call) == 2 && call[0] == 's'
		isFollowUp := len(call) == 2 && call[0] == 'f'
		if !isSteer && !isFollowUp {
			t.Fatalf("unexpected call token: %q", call)
		}
		if isFollowUp {
			seenFollowUp = true
		}
		if isSteer && seenFollowUp {
			t.Fatalf("steer executed after follow_up, priority drift detected: %v", got)
		}
	}
}

func TestCommandLoopRejectsCommandsWithoutActiveRun(t *testing.T) {
	loop := NewCommandLoop(newGatedExecutor())

	if err := loop.Steer("x"); err == nil {
		t.Fatalf("expected steer to fail when idle")
	}
	if err := loop.FollowUp("x"); err == nil {
		t.Fatalf("expected follow_up to fail when idle")
	}
	if err := loop.Abort(); err == nil {
		t.Fatalf("expected abort to fail when idle")
	}
}

func TestCommandLoopAbortPropagatesContextCancellation(t *testing.T) {
	exec := newGatedExecutor()
	loop := NewCommandLoop(exec)

	results := make(chan TurnResult, 1)
	loop.SetOnTurnEnd(func(r TurnResult) {
		results <- r
	})

	if _, err := loop.Prompt("p0"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first prompt did not start")
	}
	if err := loop.Abort(); err != nil {
		t.Fatalf("abort failed: %v", err)
	}

	select {
	case r := <-results:
		if !errors.Is(r.Err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", r.Err)
		}
	case <-time.After(time.Second):
		t.Fatalf("did not receive turn result")
	}
	waitLoopIdle(t, loop)
}
