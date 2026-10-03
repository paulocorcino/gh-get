package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

func writeMeta(t *testing.T, dir string, m meta.Meta) {
	t.Helper()
	m.SourceURL = "x"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := meta.Write(dir, m); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledSource_RecognizesDefaultDest(t *testing.T) {
	base := t.TempDir()
	writeMeta(t, filepath.Join(base, "tdd"), meta.Meta{
		Owner: "o", Repo: "r", Branch: "main", FolderPath: "skills/engineering/tdd", Commit: "c1",
	})
	e := manifest.Entry{URL: "https://github.com/o/r/blob/main/skills/engineering/tdd"}

	src, dest, ok := installedSource(e, base, false)
	if !ok {
		t.Fatal("installed entry not recognized")
	}
	if dest != filepath.Join(base, "tdd") || src.Ref != "main" || src.Path != "skills/engineering/tdd" || src.Commit != "c1" {
		t.Fatalf("dest=%q ref=%q path=%q commit=%q", dest, src.Ref, src.Path, src.Commit)
	}
}

func TestInstalledSource_RejectsMismatches(t *testing.T) {
	base := t.TempDir()
	writeMeta(t, filepath.Join(base, "a"), meta.Meta{Owner: "o", Repo: "r", Branch: "v2", FolderPath: "a"})
	// A folder named after the repo whose marker is for a sub-folder install:
	// the URL's default destination would be "x", not "r".
	writeMeta(t, filepath.Join(base, "r"), meta.Meta{Owner: "o", Repo: "r", Branch: "main", FolderPath: "x"})
	writeMeta(t, filepath.Join(base, "whole"), meta.Meta{Owner: "o", Repo: "whole", Branch: "main"})

	cases := []struct {
		name      string
		e         manifest.Entry
		allowBare bool
	}{
		{"different ref", manifest.Entry{URL: "https://github.com/o/r/tree/main/a"}, true},
		{"default dest elsewhere", manifest.Entry{URL: "https://github.com/o/r/tree/main/x"}, true},
		{"bare url while updating", manifest.Entry{URL: "https://github.com/o/whole"}, false},
		{"not installed", manifest.Entry{URL: "https://github.com/o/r/tree/main/missing"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := installedSource(tc.e, base, tc.allowBare); ok {
				t.Fatal("unexpectedly recognized")
			}
		})
	}

	if _, _, ok := installedSource(manifest.Entry{URL: "https://github.com/o/whole"}, base, true); !ok {
		t.Fatal("bare url should be recognized in download mode")
	}
}

func TestSyncListEntry_PresentEntryNeedsNoNetwork(t *testing.T) {
	base := t.TempDir()
	writeMeta(t, filepath.Join(base, "vendor", "b"), meta.Meta{
		Owner: "nobody-xyz", Repo: "no-such-repo", Branch: "feature/x", FolderPath: "skills/b", Commit: "c1",
	})
	e := manifest.Entry{URL: "https://github.com/nobody-xyz/no-such-repo/tree/feature/x/skills/b", Dest: "vendor/b"}

	// Any request for this fake repo would fail, so success proves none is made.
	dest, action, err := syncListEntry(fetch.New("", nil), e, base, t.TempDir(), nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if action != listCurrent || dest != filepath.Join(base, "vendor", "b") {
		t.Fatalf("action=%v dest=%q", action, dest)
	}
}
