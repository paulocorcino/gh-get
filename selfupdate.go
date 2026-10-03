package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// releasesURL is the project's GitHub releases page; a variable so tests can
// point it at a local server. Everything self-update needs is served here, not
// by the REST API, so it does not consume the API rate limit.
var releasesURL = "https://github.com/paulocorcino/gh-get/releases"

// maxReleaseAsset caps a downloaded release file, guarding against a runaway
// response (binaries are a few MB).
const maxReleaseAsset = 200 << 20

// runSelfUpdate replaces the running executable with the latest release.
func runSelfUpdate(force bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return selfUpdate(exe, version, runtime.GOOS, runtime.GOARCH, force)
}

// selfUpdate replaces exe (built as version current) with the latest release
// for goos/goarch. The download is verified against the release's
// checksums.txt before exe is touched. A dev build or an already-current
// version is only replaced with force.
func selfUpdate(exe, current, goos, goarch string, force bool) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	latest, err := latestRelease(client)
	if err != nil {
		return fmt.Errorf("finding the latest release: %w", err)
	}
	if current == "dev" && !force {
		return fmt.Errorf("this is a development build; use --self-update --force to replace it with %s", latest)
	}
	if current == latest && !force {
		fmt.Printf("gh-get %s is already the latest version.\n", current)
		return nil
	}

	asset := releaseAsset(goos, goarch)
	fmt.Printf("Downloading gh-get %s (%s)...\n", latest, asset)
	sums, err := fetchReleaseFile(client, latest, "checksums.txt")
	if err != nil {
		return err
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return err
	}
	bin, err := fetchReleaseFile(client, latest, asset)
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(bin); hex.EncodeToString(sum[:]) != want {
		return fmt.Errorf("checksum mismatch for %s; the binary was not replaced", asset)
	}

	if err := replaceExecutable(exe, bin); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("no write access to %s; run \"gh-get --install\" for a per-user copy, or update with the rights that installed it: %w", exe, err)
		}
		return fmt.Errorf("replacing %s: %w", exe, err)
	}
	fmt.Printf("Updated gh-get %s -> %s\n  %s\n", current, latest, exe)
	return nil
}

// latestRelease returns the tag of the latest release, read from the redirect
// of releases/latest to releases/tag/<tag> (no API request needed).
func latestRelease(client *http.Client) (string, error) {
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Get(releasesURL + "/latest")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	_, tag, ok := strings.Cut(loc, "/releases/tag/")
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || !ok || tag == "" {
		return "", fmt.Errorf("no published release found (HTTP %d)", resp.StatusCode)
	}
	return url.PathUnescape(tag)
}

// releaseAsset is the raw binary name the release workflow publishes.
func releaseAsset(goos, goarch string) string {
	name := "gh-get_" + goos + "_" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func fetchReleaseFile(client *http.Client, tag, name string) ([]byte, error) {
	u := releasesURL + "/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s from release %s: HTTP %d", name, tag, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseAsset+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReleaseAsset {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, maxReleaseAsset)
	}
	return data, nil
}

// checksumFor finds asset's SHA-256 in sha256sum output ("<hex>  <name>").
func checksumFor(sums []byte, asset string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in the release (unsupported platform?)", asset)
}

// replaceExecutable atomically swaps exe for a new binary. The new file is
// staged beside exe so the final rename stays on one filesystem. Windows
// cannot overwrite a running executable but can rename it, so the old one is
// moved aside and removed on a later run (see removeReplacedBinary).
func replaceExecutable(exe string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".gh-get-new-*")
	if err != nil {
		return err
	}
	staged := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(staged, 0o755)); err != nil {
		os.Remove(staged)
		return err
	}

	if runtime.GOOS == "windows" {
		old := exe + ".old"
		if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
			// A previous old binary is still running; pick a fresh name.
			old += "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		}
		if err := os.Rename(exe, old); err != nil {
			os.Remove(staged)
			return err
		}
		if err := os.Rename(staged, exe); err != nil {
			os.Rename(old, exe) // put the original back
			os.Remove(staged)
			return err
		}
		return nil
	}

	if err := os.Rename(staged, exe); err != nil {
		os.Remove(staged)
		return err
	}
	return nil
}

// removeReplacedBinary deletes executables a Windows self-update moved aside.
// It is best-effort: one still running (or locked) is left for the next run.
func removeReplacedBinary() {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	olds, _ := filepath.Glob(exe + ".old*")
	for _, old := range olds {
		os.Remove(old)
	}
}
