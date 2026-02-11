package main

import "testing"

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		wantCmd string
		wantErr bool
	}{
		{args: []string{"ping"}, wantCmd: "ping"},
		{args: []string{"prompt", "hello"}, wantCmd: "prompt"},
		{args: []string{"steer", "focus"}, wantCmd: "steer"},
		{args: []string{"follow_up", "next"}, wantCmd: "follow_up"},
		{args: []string{"abort"}, wantCmd: "abort"},
		{args: []string{"new"}, wantCmd: "new_session"},
		{args: []string{"switch", "sess-1"}, wantCmd: "switch_session"},
		{args: []string{"set_active_tools", "a", "b"}, wantCmd: "set_active_tools"},
		{args: []string{"prompt"}, wantErr: true},
	}

	for _, tc := range tests {
		cmd, _, err := parseArgs(tc.args)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for args=%v", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for args=%v: %v", tc.args, err)
		}
		if cmd != tc.wantCmd {
			t.Fatalf("unexpected cmd for args=%v: got=%q want=%q", tc.args, cmd, tc.wantCmd)
		}
	}
}
