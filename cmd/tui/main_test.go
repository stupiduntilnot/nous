package main

import (
	"testing"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		in      string
		wantCmd string
		wantErr bool
		wantQ   bool
	}{
		{in: "ping", wantCmd: "ping"},
		{in: "prompt hello", wantCmd: "prompt"},
		{in: "steer focus", wantCmd: "steer"},
		{in: "follow_up next", wantCmd: "follow_up"},
		{in: "abort", wantCmd: "abort"},
		{in: "new", wantCmd: "new_session"},
		{in: "switch sess-1", wantCmd: "switch_session"},
		{in: "quit", wantQ: true},
		{in: "prompt ", wantErr: true},
	}

	for _, tc := range tests {
		cmd, _, quit, err := parseInput(tc.in)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for %q", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.in, err)
		}
		if quit != tc.wantQ {
			t.Fatalf("unexpected quit flag for %q: got=%v want=%v", tc.in, quit, tc.wantQ)
		}
		if cmd != tc.wantCmd {
			t.Fatalf("unexpected cmd for %q: got=%q want=%q", tc.in, cmd, tc.wantCmd)
		}
	}
}
