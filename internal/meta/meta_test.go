package meta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Meta{
		SourceURL:  "https://github.com/o/r/tree/main/a/b",
		Owner:      "o",
		Repo:       "r",
		Branch:     "main",
		FolderPath: "a/b",
		Commit:     "abc123",
	}
	if err := Write(dir, in); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("marker not written: %v", err)
	}

	out, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out.SourceURL != in.SourceURL || out.Owner != in.Owner || out.Repo != in.Repo ||
		out.Branch != in.Branch || out.FolderPath != in.FolderPath || out.Commit != in.Commit {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, in)
	}
	if out.UpdatedAt == "" {
		t.Fatal("updated_at should be stamped on write")
	}
}

func TestRead_Missing(t *testing.T) {
	_, err := Read(t.TempDir())
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist, got %v", err)
	}
}

func TestRead_IgnoresBlankAndBadLines(t *testing.T) {
	dir := t.TempDir()
	content := "source_url=u\n\ngarbage-no-eq\nowner=o\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.SourceURL != "u" || m.Owner != "o" {
		t.Fatalf("got %+v", m)
	}
}
