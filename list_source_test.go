package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
)

func TestIsRemoteList(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/o/r/blob/main/gh-get.txt": true,
		"http://github.com/o/r/blob/main/gh-get.txt":  true,
		"gh-get.txt":         false,
		"./lists/tools.txt":  false,
		`C:\lists\tools.txt`: false,
	}
	for arg, want := range cases {
		if got := isRemoteList(arg); got != want {
			t.Errorf("isRemoteList(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestCheckListDest_RemoteListHasNoListDir(t *testing.T) {
	cwd := t.TempDir()
	dest := filepath.Join(cwd, "sub")
	if err := checkListDest(dest, "", cwd, nil); err != nil {
		t.Fatalf("destination under cwd refused for a remote list: %v", err)
	}
	if err := checkListDest(cwd, "", cwd, nil); !errors.Is(err, ErrDestContainsCwd) {
		t.Fatalf("err = %v, want ErrDestContainsCwd", err)
	}
	if err := checkListDest(cwd, cwd, cwd, nil); !errors.Is(err, ErrDestContainsList) {
		t.Fatalf("err = %v, want ErrDestContainsList", err)
	}
}

func TestFreeze_RefusesRemoteList(t *testing.T) {
	cl := commandLine{Positional: []string{"freeze"}, ListFile: "https://github.com/o/r/blob/main/gh-get.txt"}
	if _, err := cl.freezeListFile(); !errors.Is(err, ErrFreezeRemote) {
		t.Fatalf("err = %v, want ErrFreezeRemote", err)
	}
}

func TestRun_UpgradeAndOldNameShareArgumentRules(t *testing.T) {
	for _, flag := range []string{"--upgrade", "--self-update"} {
		err := run([]string{flag, "--bogus"}, new(bytes.Buffer), new(bytes.Buffer))
		if !errors.Is(err, ErrUpgradeArgs) {
			t.Errorf("%s: err = %v, want ErrUpgradeArgs", flag, err)
		}
	}
}
