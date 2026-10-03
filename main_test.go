package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/meta"
)

func putMarker(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, meta.FileName), []byte("source_url=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverRecursive_FindsRootAndSortsDeepestFirst(t *testing.T) {
	root := t.TempDir()
	putMarker(t, root)
	putMarker(t, filepath.Join(root, "b"))
	putMarker(t, filepath.Join(root, "a", "deep"))

	got := discoverRecursive(root)
	wantTargets := []string{
		filepath.Join(root, "a", "deep"),
		filepath.Join(root, "b"),
	}
	if !reflect.DeepEqual(got.Targets, wantTargets) {
		t.Fatalf("targets = %#v, want %#v", got.Targets, wantTargets)
	}
	if !reflect.DeepEqual(got.Skipped, []string{root}) {
		t.Fatalf("skipped = %#v, want root", got.Skipped)
	}
	if len(got.Errors) != 0 {
		t.Fatalf("unexpected scan errors: %#v", got.Errors)
	}
}

func TestDiscoverRecursive_OnlyLeavesOfNestedInstallationsAreTargets(t *testing.T) {
	root := t.TempDir()
	middle := filepath.Join(root, "middle")
	leaf := filepath.Join(middle, "leaf")
	putMarker(t, root)
	putMarker(t, middle)
	putMarker(t, leaf)

	got := discoverRecursive(root)
	if !reflect.DeepEqual(got.Targets, []string{leaf}) {
		t.Fatalf("targets = %#v, want leaf only", got.Targets)
	}
	if !reflect.DeepEqual(got.Skipped, []string{middle, root}) {
		t.Fatalf("skipped = %#v, want middle then root", got.Skipped)
	}
}

func TestDiscoverRecursive_DoesNotFollowDirectorySymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	putMarker(t, outside)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}

	got := discoverRecursive(root)
	if len(got.Targets) != 0 || len(got.Skipped) != 0 {
		t.Fatalf("followed directory symlink: %#v", got)
	}
}

func TestUpdateAll_ContinuesAfterFailure(t *testing.T) {
	var called []string
	attempts := updateAll([]string{"one", "two"}, func(path string) (updateStatus, error) {
		called = append(called, path)
		if path == "one" {
			return updateNotManaged, errors.New("boom")
		}
		return updateCurrent, nil
	})

	if !reflect.DeepEqual(called, []string{"one", "two"}) {
		t.Fatalf("called = %#v", called)
	}
	if len(attempts) != 2 || attempts[0].Err == nil || attempts[1].Status != updateCurrent {
		t.Fatalf("attempts = %#v", attempts)
	}
}

func TestRun_RejectsRecursiveWithRef(t *testing.T) {
	err := run([]string{"update", "-r", "--ref", "main"})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_RejectsRecursiveOutsideUpdate(t *testing.T) {
	err := run([]string{"https://github.com/o/r", "--recursive"})
	if err == nil || !strings.Contains(err.Error(), "only valid with update") {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_RecursiveNoInstallationsSucceeds(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"update", "--recursive", "--token", "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAt_InvalidMarkerFailsBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	if err := meta.Write(dir, meta.Meta{SourceURL: "https://github.com/o/r"}); err != nil {
		t.Fatal(err)
	}
	_, err := updateAt(fetch.New("test"), dir, "", false)
	if err == nil || !strings.Contains(err.Error(), "invalid "+meta.FileName) {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanListEntry(t *testing.T) {
	src := ghurl.Build("o", "r", "main", "skills/a", "sha2")
	same := meta.Meta{SourceURL: "x", Owner: "O", Repo: "r", Branch: "main", FolderPath: "skills/a", Commit: "sha1"}
	other := meta.Meta{SourceURL: "x", Owner: "o", Repo: "r", Branch: "main", FolderPath: "skills/b", Commit: "sha1"}
	upToDate := same
	upToDate.Commit = "sha2"
	otherRef := upToDate
	otherRef.Branch = "dev"

	cases := []struct {
		name             string
		st               destState
		refresh, force   bool
		want             listAction
		wantErrSubstring string
	}{
		{name: "absent installs", st: destState{}, want: listInstall},
		{name: "absent installs on update", st: destState{}, refresh: true, want: listInstall},
		{name: "download leaves same source alone", st: destState{Exists: true, Managed: true, Meta: same}, want: listCurrent},
		{name: "update refreshes new commit", st: destState{Exists: true, Managed: true, Meta: same}, refresh: true, want: listUpdate},
		{name: "update skips same commit", st: destState{Exists: true, Managed: true, Meta: upToDate}, refresh: true, want: listCurrent},
		{name: "update switches ref", st: destState{Exists: true, Managed: true, Meta: otherRef}, refresh: true, want: listUpdate},
		{name: "other source refused", st: destState{Exists: true, Managed: true, Meta: other}, refresh: true, wantErrSubstring: "different gh-get download"},
		{name: "other source forced", st: destState{Exists: true, Managed: true, Meta: other}, force: true, want: listInstall},
		{name: "unmanaged refused", st: destState{Exists: true}, wantErrSubstring: "not downloaded by gh-get"},
		{name: "unmanaged forced", st: destState{Exists: true}, force: true, want: listInstall},
	}
	for _, c := range cases {
		got, err := planListEntry(c.st, src, c.refresh, c.force)
		if c.wantErrSubstring != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErrSubstring) {
				t.Errorf("%s: err = %v", c.name, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestRun_FileRejectsIncompatibleArgs(t *testing.T) {
	for _, args := range [][]string{
		{"-F", "gh-get.txt", "https://github.com/o/r"},
		{"--file", "gh-get.txt", "--ref", "main"},
		{"update", "--file=gh-get.txt", "-r"},
	} {
		if err := run(args); err == nil {
			t.Errorf("run(%q) succeeded, want error", args)
		}
	}
}

func TestRun_EmptyListFileSucceeds(t *testing.T) {
	list := filepath.Join(t.TempDir(), "gh-get.txt")
	if err := os.WriteFile(list, []byte("# nothing yet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-F", list, "--token", "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestDestState_ReadsMarker(t *testing.T) {
	root := t.TempDir()
	if st, err := readDestState(filepath.Join(root, "missing")); err != nil || st.Exists {
		t.Fatalf("missing: %#v, %v", st, err)
	}
	if st, err := readDestState(root); err != nil || !st.Exists || st.Managed {
		t.Fatalf("unmanaged: %#v, %v", st, err)
	}
	putMarker(t, root)
	if st, err := readDestState(root); err != nil || !st.Managed {
		t.Fatalf("managed: %#v, %v", st, err)
	}
}
