package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

func inst(base, rel, owner, repo, branch, path string) installation {
	return installation{
		Dir:  filepath.Join(base, filepath.FromSlash(rel)),
		Meta: meta.Meta{SourceURL: "x", Owner: owner, Repo: repo, Branch: branch, FolderPath: path},
	}
}

func TestMergeFreeze_KeepsChangesAndAppends(t *testing.T) {
	base := t.TempDir()
	text := "# my skills\n" +
		"https://github.com/o/r/blob/main/skills/a\n" +
		"https://github.com/o/r/tree/main/skills/b  vendor/b  # keep me\n" +
		"https://github.com/o/whole\n" +
		"https://github.com/o/r/tree/main/skills/not-downloaded\n"
	entries, err := manifest.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	insts := []installation{
		inst(base, "a", "o", "r", "main", "skills/a"),       // listed (blob == tree)
		inst(base, "vendor/b", "o", "r", "v2", "skills/b"),  // switched ref
		inst(base, "whole", "o", "whole", "main", ""),       // bare URL stays bare
		inst(base, "tools/c", "o", "r", "main", "skills/c"), // new, non-default dest
		inst(base, "d", "x", "y", "feature/z", "d"),         // new, default dest
	}

	out, results := mergeFreeze(lines, entries, base, insts)
	want := []string{
		"# my skills",
		"https://github.com/o/r/blob/main/skills/a",
		"https://github.com/o/r/tree/v2/skills/b  vendor/b  # keep me",
		"https://github.com/o/whole",
		"https://github.com/o/r/tree/main/skills/not-downloaded",
		"https://github.com/x/y/tree/feature/z/d",
		"https://github.com/o/r/tree/main/skills/c  tools/c",
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out =\n%s\nwant\n%s", strings.Join(out, "\n"), strings.Join(want, "\n"))
	}
	statuses := map[string]freezeStatus{}
	for _, r := range results {
		statuses[filepath.Base(r.Inst.Dir)] = r.Status
	}
	wantStatus := map[string]freezeStatus{"a": freezeKept, "b": freezeChanged, "whole": freezeKept, "c": freezeAdded, "d": freezeAdded}
	if !reflect.DeepEqual(statuses, wantStatus) {
		t.Fatalf("statuses = %v, want %v", statuses, wantStatus)
	}
}

func TestRunFreeze_CreatesThenKeepsListFile(t *testing.T) {
	root := t.TempDir()
	for _, m := range []meta.Meta{
		{SourceURL: "x", Owner: "o", Repo: "r", Branch: "main", FolderPath: "skills/a"},
		{SourceURL: "x", Owner: "o", Repo: "r", Branch: "main", FolderPath: "skills/b"},
	} {
		dir := filepath.Join(root, "sub", filepath.Base(m.FolderPath))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := meta.Write(dir, m); err != nil {
			t.Fatal(err)
		}
	}
	list := filepath.Join(root, manifest.DefaultFile)

	if err := runFreeze(list); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := manifest.Parse(strings.NewReader(string(first)))
	if err != nil || len(entries) != 2 || entries[0].Dest != "sub/a" || entries[1].Dest != "sub/b" {
		t.Fatalf("entries = %#v, err %v\n%s", entries, err, first)
	}

	if err := runFreeze(list); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(list)
	if string(first) != string(second) {
		t.Fatalf("second freeze changed the file:\n%s", second)
	}
}

func TestRun_FreezeRejectsOtherFlags(t *testing.T) {
	if err := run([]string{"freeze", "--force"}); err == nil {
		t.Fatal("want error")
	}
}
