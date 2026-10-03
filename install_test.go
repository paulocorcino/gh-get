package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellProfile(t *testing.T) {
	home := "/home/u"
	dir := "/home/u/.local/bin"
	cases := []struct {
		shell, zdotdir, goos, wantFile string
	}{
		{"/bin/zsh", "", "darwin", "/home/u/.zshrc"},
		{"/usr/bin/zsh", "/home/u/.config/zsh", "linux", "/home/u/.config/zsh/.zshrc"},
		{"/bin/bash", "", "linux", "/home/u/.bashrc"},
		{"/bin/bash", "", "darwin", "/home/u/.bash_profile"},
		{"/usr/bin/fish", "", "linux", "/home/u/.config/fish/conf.d/gh-get.fish"},
		{"/bin/sh", "", "linux", "/home/u/.profile"},
		{"", "", "linux", "/home/u/.profile"},
	}
	for _, tc := range cases {
		got := shellProfile(tc.shell, home, tc.zdotdir, tc.goos, dir)
		if got.File != filepath.Join(filepath.FromSlash(tc.wantFile)) {
			t.Errorf("shell %q on %s: file = %q, want %q", tc.shell, tc.goos, got.File, tc.wantFile)
		}
	}

	posix := shellProfile("/bin/bash", home, "", "linux", "/home/o'b/bin").Line
	want := `case ":$PATH:" in *:'/home/o'\''b/bin':*) ;; *) export PATH='/home/o'\''b/bin':"$PATH" ;; esac`
	if posix != want {
		t.Fatalf("posix line = %s\nwant          %s", posix, want)
	}
}

func TestAddToProfile_AppendsOnce(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", ".zshrc")
	p := profileEdit{File: file, Line: "export PATH=x"}

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("alias ll='ls -l'"), 0o644); err != nil { // no trailing newline
		t.Fatal(err)
	}
	added, err := addToProfile(p)
	if err != nil || !added {
		t.Fatalf("first add: added=%v err=%v", added, err)
	}
	added, err = addToProfile(p)
	if err != nil || added {
		t.Fatalf("second add: added=%v err=%v, want no-op", added, err)
	}

	got, _ := os.ReadFile(file)
	want := "alias ll='ls -l'\n\n" + profileMarker + "\nexport PATH=x\n"
	if string(got) != want {
		t.Fatalf("profile =\n%q\nwant\n%q", got, want)
	}
}

func TestAddToProfile_CreatesMissingFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".config", "fish", "conf.d", "gh-get.fish")
	if added, err := addToProfile(profileEdit{File: file, Line: "set -gx PATH x $PATH"}); err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if got, _ := os.ReadFile(file); !strings.Contains(string(got), "set -gx PATH x $PATH") {
		t.Fatalf("file = %q", got)
	}
}

func TestPathContains(t *testing.T) {
	sep := ":"
	dir := "/home/u/.local/bin"
	other := "/usr/bin"
	if runtime.GOOS == "windows" {
		sep = ";"
		dir = `C:\Users\u\AppData\Local\Programs\gh-get`
		other = `C:\Windows\System32`
	}

	tests := []struct {
		name    string
		pathEnv string
		dir     string
		want    bool
	}{
		{"empty", "", dir, false},
		{"present", strings.Join([]string{other, dir}, sep), dir, true},
		{"absent", other, dir, false},
		{"trailing separator", dir + sep, dir, true},
		{"unclean entry", strings.Join([]string{other, dir + sep + "."}, sep), dir, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathContains(tc.pathEnv, tc.dir); got != tc.want {
				t.Errorf("pathContains(%q, %q) = %v, want %v", tc.pathEnv, tc.dir, got, tc.want)
			}
		})
	}
}

func TestPathContainsCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive PATH matching only applies on Windows")
	}
	dir := `C:\Users\u\AppData\Local\Programs\gh-get`
	if !pathContains(strings.ToUpper(dir), dir) {
		t.Errorf("expected case-insensitive match on Windows")
	}
}

func TestInstallBinName(t *testing.T) {
	got := installBinName()
	want := "gh-get"
	if runtime.GOOS == "windows" {
		want = "gh-get.exe"
	}
	if got != want {
		t.Errorf("installBinName() = %q, want %q", got, want)
	}
}
