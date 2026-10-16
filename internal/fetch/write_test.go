package fetch

import (
	"os"
	"path/filepath"
	"testing"
)

// buildTree writes a small source tree used by the publish/clear tests.
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "bravo")
	return root
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPublishTree_CopiesNested(t *testing.T) {
	src := buildTree(t)
	dst := t.TempDir()

	if err := publishTree(src, dst, nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, "a.txt")); got != "alpha" {
		t.Fatalf("a.txt = %q", got)
	}
	if got := readFile(t, filepath.Join(dst, "sub", "b.txt")); got != "bravo" {
		t.Fatalf("sub/b.txt = %q", got)
	}
}

func TestPublishTree_MergeKeepsNonColliding(t *testing.T) {
	src := buildTree(t)
	dst := t.TempDir()
	mustWrite(t, filepath.Join(dst, "keep.txt"), "kept")
	mustWrite(t, filepath.Join(dst, "a.txt"), "old")

	if err := publishTree(src, dst, nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, "keep.txt")); got != "kept" {
		t.Fatalf("merge wiped a non-colliding file: keep.txt = %q", got)
	}
	if got := readFile(t, filepath.Join(dst, "a.txt")); got != "alpha" {
		t.Fatalf("merge did not overwrite collision: a.txt = %q", got)
	}
}

func TestClearContents_KeepsDirRemovesEntries(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "x.txt"), "x")
	mustWrite(t, filepath.Join(dir, "nested", "y.txt"), "y")

	if err := clearContents(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("clearContents must keep the directory itself: %v", err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("expected empty dir, got %d entries", len(ents))
	}
}

func TestPruneStale_RemovesOnlyTrackedStaleFiles(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"keep.md", "old/gone.md", "mine/notes.txt", "shared/old.md", "shared/user.txt"} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prev := []string{"keep.md", "old/gone.md", "shared/old.md", "../escape.md"}
	if err := pruneStale(dir, prev, []string{"keep.md"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"keep.md", "mine/notes.txt", "shared/user.txt"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("%s should survive: %v", p, err)
		}
	}
	for _, p := range []string{"old/gone.md", "old", "shared/old.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", p)
		}
	}
}
