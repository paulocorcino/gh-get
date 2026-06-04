package main

import (
	"runtime"
	"strings"
	"testing"
)

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
