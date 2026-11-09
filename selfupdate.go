package main

import (
	"bufio"
	"bytes"
	"context"
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
var releasesURL = "https://github.com/paulocorcino/gh-get/releases" //nolint:gochecknoglobals // test seam: tests point it at a local server

// Errors of --upgrade; each prints exactly its text.
var (
	ErrDevBuild         = errors.New("this is a development build")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrNoRelease        = errors.New("no published release found")
	ErrHTTP             = errors.New("HTTP")
	ErrAssetTooLarge    = errors.New("exceeds")
	ErrNoChecksum       = errors.New("no checksum")
)

// maxReleaseAsset caps a downloaded release file, guarding against a runaway
// response (binaries are a few MB).
const maxReleaseAsset = 200 << 20

// runSelfUpdate replaces the running executable with the latest release.
func runSelfUpdate(force bool, stdout io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return selfUpdate(exe, version, releaseAsset(runtime.GOOS, runtime.GOARCH), force, stdout)
}

// selfUpdate replaces exe (built as version current) with the release file
// asset of the latest release, printing progress to stdout. The download is
// verified against the release's checksums.txt before exe is touched. A dev
// build or an already-current version is only replaced with force.
func selfUpdate(exe, current, asset string, force bool, stdout io.Writer) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	ctx := context.Background()
	latest, err := latestRelease(ctx, client)
	if err != nil {
		return fmt.Errorf("finding the latest release: %w", err)
	}
	if current == "dev" && !force {
		return fmt.Errorf("%w; use --upgrade --force to replace it with %s", ErrDevBuild, latest)
	}
	if current == latest && !force {
		outf(stdout, "gh-get %s is already the latest version.\n", current)
		return nil
	}

	outf(stdout, "Downloading gh-get %s (%s)...\n", latest, asset)
	bin, err := fetchVerifiedAsset(ctx, client, latest, asset)
	if err != nil {
		return err
	}

	if err := replaceExecutable(exe, bin); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("no write access to %s; run \"gh-get --install\" for a per-user copy, or update with the rights that installed it: %w", exe, err)
		}
		return fmt.Errorf("replacing %s: %w", exe, err)
	}
	outf(stdout, "Updated gh-get %s -> %s\n  %s\n", current, latest, exe)
	return nil
}

// fetchVerifiedAsset downloads the release file asset of release tag and
// checks it against the release's checksums.txt.
func fetchVerifiedAsset(ctx context.Context, client *http.Client, tag, asset string) ([]byte, error) {
	sums, err := fetchReleaseFile(ctx, client, tag, "checksums.txt")
	if err != nil {
		return nil, fmt.Errorf("fetching checksums: %w", err)
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return nil, fmt.Errorf("verifying: %w", err)
	}
	bin, err := fetchReleaseFile(ctx, client, tag, asset)
	if err != nil {
		return nil, fmt.Errorf("fetching the binary: %w", err)
	}
	if sum := sha256.Sum256(bin); hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf("%w for %s; the binary was not replaced", ErrChecksumMismatch, asset)
	}
	return bin, nil
}

// latestRelease returns the tag of the latest release, read from the redirect
// of releases/latest to releases/tag/<tag> (no API request needed).
func latestRelease(ctx context.Context, client *http.Client) (string, error) {
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL+"/latest", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("requesting the latest release: %w", err)
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting the latest release: %w", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		warn("closing the response: " + closeErr.Error())
	}
	loc := resp.Header.Get("Location")
	_, tag, ok := strings.Cut(loc, "/releases/tag/")
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || !ok || tag == "" {
		return "", fmt.Errorf("%w (HTTP %d)", ErrNoRelease, resp.StatusCode)
	}
	unescaped, err := url.PathUnescape(tag)
	if err != nil {
		return "", fmt.Errorf("reading the release tag: %w", err)
	}
	return unescaped, nil
}

// releaseAsset is the raw binary name the release workflow publishes.
func releaseAsset(goos, goarch string) string {
	name := "gh-get_" + goos + "_" + goarch
	if goos == goosWindows {
		name += ".exe"
	}
	return name
}

func fetchReleaseFile(ctx context.Context, client *http.Client, tag, name string) ([]byte, error) {
	u := releasesURL + "/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	defer closeOrWarn(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s from release %s: %w %d", name, tag, ErrHTTP, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseAsset+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	if len(data) > maxReleaseAsset {
		return nil, fmt.Errorf("%s %w %d bytes", name, ErrAssetTooLarge, maxReleaseAsset)
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
	return "", fmt.Errorf("%w for %s in the release (unsupported platform?)", ErrNoChecksum, asset)
}

// replaceExecutable atomically swaps exe for a new binary. The new file is
// staged beside exe so the final rename stays on one filesystem. Windows
// cannot overwrite a running executable but can rename it, so the old one is
// moved aside and removed on a later run (see removeReplacedBinary).
func replaceExecutable(exe string, data []byte) error {
	staged, err := stageBinary(exe, data)
	if err != nil {
		return err
	}
	if runtime.GOOS == goosWindows {
		return swapRunningBinary(exe, staged)
	}
	if err := os.Rename(staged, exe); err != nil {
		removeOrWarn(staged)
		return fmt.Errorf("installing the new binary: %w", err)
	}
	return nil
}

// stageBinary writes data to a new executable file beside exe and returns its
// path; on a failure nothing is left behind.
func stageBinary(exe string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".gh-get-new-*")
	if err != nil {
		return "", fmt.Errorf("staging the new binary: %w", err)
	}
	staged := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(staged, 0o755)); err != nil {
		removeOrWarn(staged)
		return "", fmt.Errorf("staging the new binary: %w", err)
	}
	return staged, nil
}

// swapRunningBinary puts the staged binary at exe on Windows, which cannot
// overwrite a running executable but can rename it: the old one is moved aside
// (removed on a later run) and put back if the swap fails.
func swapRunningBinary(exe, staged string) error {
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// A previous old binary is still running; pick a fresh name.
		old += "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if err := os.Rename(exe, old); err != nil {
		removeOrWarn(staged)
		return fmt.Errorf("moving the old binary aside: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		if rerr := os.Rename(old, exe); rerr != nil { // put the original back
			warn("restoring " + exe + ": " + rerr.Error())
		}
		removeOrWarn(staged)
		return fmt.Errorf("installing the new binary: %w", err)
	}
	return nil
}

// removeOrWarn deletes a staged file on a failure path, where the error being
// returned matters more than a leftover file.
func removeOrWarn(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		warn("removing " + path + ": " + err.Error())
	}
}

// removeReplacedBinary deletes executables a Windows self-update moved aside.
// It is best-effort: one still running (or locked) is left for the next run.
func removeReplacedBinary() {
	if runtime.GOOS != goosWindows {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	olds, err := filepath.Glob(exe + ".old*")
	if err != nil {
		return
	}
	for _, old := range olds {
		os.Remove(old) //nolint:errcheck,gosec // best effort: a binary still running or locked is left for the next run
	}
}
