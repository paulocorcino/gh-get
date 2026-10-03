package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeReleases serves a releases page whose latest tag is v9.9.9 with one
// asset; checksum overrides the published SHA-256 when non-empty.
func fakeReleases(t *testing.T, asset, body, checksum string) {
	t.Helper()
	if checksum == "" {
		sum := sha256.Sum256([]byte(body))
		checksum = hex.EncodeToString(sum[:])
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v9.9.9", http.StatusFound)
		case "/releases/download/v9.9.9/checksums.txt":
			w.Write([]byte("deadbeef  other_file\n" + checksum + "  " + asset + "\n"))
		case "/releases/download/v9.9.9/" + asset:
			w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := releasesURL
	releasesURL = srv.URL + "/releases"
	t.Cleanup(func() { releasesURL = old })
}

func fakeExe(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "gh-get")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestSelfUpdate_ReplacesWithVerifiedBinary(t *testing.T) {
	asset := releaseAsset("linux", "amd64")
	fakeReleases(t, asset, "new binary", "")
	exe := fakeExe(t)

	if err := selfUpdate(exe, "v1.0.0", "linux", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new binary" {
		t.Fatalf("exe = %q, want the new binary", got)
	}
	// Only the swapped-in binary (and, on Windows, the moved-aside one) remain.
	ents, _ := os.ReadDir(filepath.Dir(exe))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".gh-get-new-") {
			t.Fatalf("staged file left behind: %s", e.Name())
		}
	}
}

func TestSelfUpdate_ChecksumMismatchKeepsBinary(t *testing.T) {
	asset := releaseAsset("linux", "amd64")
	fakeReleases(t, asset, "tampered", strings.Repeat("0", 64))
	exe := fakeExe(t)

	err := selfUpdate(exe, "v1.0.0", "linux", "amd64", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatalf("exe = %q, must be untouched", got)
	}
}

func TestSelfUpdate_SkipsCurrentAndDevWithoutForce(t *testing.T) {
	asset := releaseAsset("linux", "amd64")
	fakeReleases(t, asset, "new binary", "")
	exe := fakeExe(t)

	if err := selfUpdate(exe, "v9.9.9", "linux", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if err := selfUpdate(exe, "dev", "linux", "amd64", false); err == nil {
		t.Fatal("dev build replaced without --force")
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatalf("exe = %q, must be untouched", got)
	}

	if err := selfUpdate(exe, "dev", "linux", "amd64", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new binary" {
		t.Fatalf("exe = %q, --force must replace", got)
	}
}

func TestSelfUpdate_UnknownPlatform(t *testing.T) {
	fakeReleases(t, releaseAsset("linux", "amd64"), "new binary", "")
	exe := fakeExe(t)
	err := selfUpdate(exe, "v1.0.0", "plan9", "mips", false)
	if err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("err = %v, want missing checksum", err)
	}
}

func TestReleaseAsset(t *testing.T) {
	if got := releaseAsset("windows", "arm64"); got != "gh-get_windows_arm64.exe" {
		t.Fatalf("got %q", got)
	}
	if got := releaseAsset("darwin", "arm64"); got != "gh-get_darwin_arm64" {
		t.Fatalf("got %q", got)
	}
}
