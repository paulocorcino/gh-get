package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/paulocorcino/gh-get/internal/fetch"
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
