package builtins

import (
	"path/filepath"
	"testing"
)

func TestResolveToolPathExpandsEnvAndJoinsRelative(t *testing.T) {
	base := t.TempDir()
	root := t.TempDir()
	t.Setenv("NOUS_BUILTINS_ROOT", root)

	got := resolveToolPath(base, "$NOUS_BUILTINS_ROOT/data.txt")
	want := filepath.Join(root, "data.txt")
	if got != want {
		t.Fatalf("unexpected env-expanded path: got=%q want=%q", got, want)
	}

	got = resolveToolPath(base, "rel/data.txt")
	want = filepath.Join(base, "rel", "data.txt")
	if got != want {
		t.Fatalf("unexpected relative path: got=%q want=%q", got, want)
	}
}

func TestResolveToolPathExpandsHome(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := resolveToolPath(base, "~/notes.md")
	want := filepath.Join(home, "notes.md")
	if got != want {
		t.Fatalf("unexpected home-expanded path: got=%q want=%q", got, want)
	}
}
