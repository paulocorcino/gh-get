// Command gh-get downloads a single folder from a GitHub repository without
// requiring git, and can update a previously downloaded folder in place.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	destpolicy "github.com/paulocorcino/gh-get/internal/dest"
	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

// Usage errors of the command line; each prints exactly its text.
var (
	ErrInstallArgs         = errors.New("--install only accepts --no-modify-path")
	ErrSelfUpdateArgs      = errors.New("--self-update only accepts --force")
	ErrTokenValue          = errors.New("--token requires a value")
	ErrMissingValue        = errors.New("requires a value")
	ErrUnknownOption       = errors.New("unknown option")
	ErrUnexpectedArg       = errors.New("unexpected argument")
	ErrFreezeArgs          = errors.New("freeze only accepts --file")
	ErrRecursiveRef        = errors.New("--recursive cannot be combined with --ref or --branch")
	ErrFileCombined        = errors.New("--file cannot be combined with --recursive, --ref or --branch")
	ErrRecursiveUpdateOnly = errors.New("--recursive is only valid with update")
	ErrFileRef             = errors.New("--file cannot be combined with --ref or --branch (put the ref in each URL)")
	ErrMissingURL          = errors.New("missing GitHub URL")
	ErrRefNotFound         = errors.New("ref not found")
	ErrRefGone             = errors.New("ref no longer exists")
	ErrRecursiveFailed     = errors.New("recursive update operation(s) failed")
	ErrInvalidMarker       = errors.New("invalid " + meta.FileName + ": owner, repo, and branch are required")
)

const usage = `gh-get - download a folder (or a whole repo) from GitHub

Usage:
  gh-get <github-url> [destination] [--force] [--ref REF] [--token TOKEN]
  gh-get -F FILE [--force]
  gh-get update [--ref REF]
  gh-get update -r | --recursive
  gh-get update -F FILE
  gh-get freeze [-F FILE]
  gh-get --install [--no-modify-path]
  gh-get --self-update [--force]
  gh-get --version | --help

Description:
  Downloads a folder from a GitHub repo (no git required), or the whole repo
  when the URL points at its root. A hidden .gh-get-source file records the
  origin so you can later run "gh-get update" from inside the folder to re-pull.

  A list file (like pip's requirements.txt) downloads many folders at once:
  one GitHub URL per line, optionally followed by a destination (relative to
  the list file); "#" starts a comment. "gh-get -F FILE" downloads the missing
  entries, "gh-get update -F FILE" also updates the existing ones. Running
  "gh-get update" in a directory with a gh-get.txt (and no .gh-get-source)
  uses that file automatically. "gh-get freeze" scans the current directory
  for downloaded folders and creates or updates gh-get.txt to list them.

URL formats:
  https://github.com/OWNER/REPO                       (whole repo, default branch)
  https://github.com/OWNER/REPO/tree/BRANCH           (whole repo at BRANCH)
  https://github.com/OWNER/REPO/tree/BRANCH/PATH/TO/FOLDER
  https://github.com/OWNER/REPO/blob/BRANCH/PATH/TO/FOLDER

Examples:
  # Download one folder
  gh-get https://github.com/octo-org/sample-repo/tree/main/docs/guide

  # Same folder into ./guide, overwriting if it already exists
  gh-get https://github.com/octo-org/sample-repo/tree/main/docs/guide ./guide --force

  # Pick a different branch/tag than the URL
  gh-get https://github.com/octo-org/sample-repo/tree/main/docs/guide --ref v2.0.0

  # Download the whole repo (default branch)
  gh-get https://github.com/octo-org/sample-repo

  # Re-pull the latest content from inside a downloaded folder
  cd ./guide && gh-get update

  # Download / update everything listed in gh-get.txt
  gh-get -F gh-get.txt
  gh-get update

  # Record every folder already downloaded under the current dir in gh-get.txt
  gh-get freeze

Options:
      --install      Copy gh-get into a per-user bin dir and put it on your
                     PATH (no admin required): the user PATH on Windows, your
                     shell profile on Linux/macOS (--no-modify-path: only print
                     the line to add)
      --self-update  Replace this gh-get with the latest GitHub release
                     (checksum-verified; --force also reinstalls the same
                     version or replaces a dev build)
  -f, --force        Overwrite the destination if it already exists
  -F, --file FILE    Download (or, with "update", update) every entry of a
                     list file such as gh-get.txt
      --ref REF      Branch, tag or commit to use; overrides the ref in the URL.
                     With "update", switches the folder to this ref.
  -r, --recursive    With "update", update every gh-get folder at or below the
                     current directory (cannot be combined with --ref)
      --token TOKEN  GitHub token (else $GITHUB_TOKEN / $GH_TOKEN; optional)
  -h, --help         Show this help
  -v, --version      Show version
`

func main() {
	removeReplacedBinary()
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1) //nolint:forbidigo // process exit code; main returns nothing
	}
}

// run executes the command line args, printing to stdout and stderr; the
// caller turns a returned error into the exit code.
func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		outf(stdout, "%s", usage)
		return nil
	}

	switch args[0] {
	case "-h", "--help":
		outf(stdout, "%s", usage)
		return nil
	case "-v", "--version":
		outln(stdout, "gh-get "+version)
		return nil
	case "--install":
		modifyPath := true
		for _, a := range args[1:] {
			if a != "--no-modify-path" {
				return ErrInstallArgs
			}
			modifyPath = false
		}
		return runInstall(modifyPath, stdout)
	case "--self-update":
		force := false
		for _, a := range args[1:] {
			if a != "-f" && a != "--force" {
				return ErrSelfUpdateArgs
			}
			force = true
		}
		return runSelfUpdate(force, stdout)
	}

	var (
		force      bool
		recursive  bool
		tokenFlag  string
		refFlag    string
		listFile   string
		positional []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-f" || a == "--force":
			force = true
		case a == "-r" || a == "--recursive":
			recursive = true
		case a == "--token":
			if i+1 >= len(args) {
				return ErrTokenValue
			}
			i++
			tokenFlag = args[i]
		case strings.HasPrefix(a, "--token="):
			tokenFlag = strings.TrimPrefix(a, "--token=")
		case a == "--ref" || a == "--branch":
			if i+1 >= len(args) {
				return fmt.Errorf("%s %w", a, ErrMissingValue)
			}
			i++
			refFlag = args[i]
		case a == "-F" || a == "--file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s %w", a, ErrMissingValue)
			}
			i++
			listFile = args[i]
		case strings.HasPrefix(a, "--file="):
			listFile = strings.TrimPrefix(a, "--file=")
		case strings.HasPrefix(a, "--ref="):
			refFlag = strings.TrimPrefix(a, "--ref=")
		case strings.HasPrefix(a, "--branch="):
			refFlag = strings.TrimPrefix(a, "--branch=")
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("%w: %s", ErrUnknownOption, a)
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) > 0 && positional[0] == "freeze" {
		if len(positional) > 1 {
			return fmt.Errorf("%w: %s", ErrUnexpectedArg, positional[1])
		}
		if force || recursive || refFlag != "" {
			return ErrFreezeArgs
		}
		if listFile == "" {
			listFile = manifest.DefaultFile
		}
		return runFreeze(listFile, stdout)
	}
	if len(positional) > 0 && positional[0] == "update" {
		if len(positional) > 1 {
			return fmt.Errorf("%w: %s", ErrUnexpectedArg, positional[1])
		}
		if recursive && refFlag != "" {
			return ErrRecursiveRef
		}
		if listFile != "" && (recursive || refFlag != "") {
			return ErrFileCombined
		}
		token := resolveToken(tokenFlag)
		if listFile != "" {
			return runList(listRequest{Path: listFile, Token: token, Refresh: true, Force: force}, stdout, stderr)
		}
		if recursive {
			return runRecursiveUpdate(token, stdout, stderr)
		}
		return runUpdate(token, refFlag, stdout, stderr)
	}
	if recursive {
		return ErrRecursiveUpdateOnly
	}
	if listFile != "" {
		if len(positional) > 0 {
			return fmt.Errorf("%w with --file: %s", ErrUnexpectedArg, positional[0])
		}
		if refFlag != "" {
			return ErrFileRef
		}
		return runList(listRequest{Path: listFile, Token: resolveToken(tokenFlag), Force: force}, stdout, stderr)
	}

	if len(positional) == 0 {
		outf(stdout, "%s", usage)
		return ErrMissingURL
	}
	if len(positional) > 2 {
		return fmt.Errorf("%w: %s", ErrUnexpectedArg, positional[2])
	}
	token := resolveToken(tokenFlag)

	req := downloadRequest{URL: positional[0], Force: force, Token: token, Ref: refFlag}
	if len(positional) == 2 {
		req.Dest = positional[1]
	}
	return runDownload(req, stdout, stderr)
}

// downloadRequest is one download asked for on the command line.
type downloadRequest struct {
	URL   string
	Dest  string // "" names the destination after the folder (or the repo)
	Force bool
	Token string
	Ref   string // overrides the ref in the URL when set
}

func runDownload(req downloadRequest, stdout, stderr io.Writer) error {
	url, dest, force, refOverride := req.URL, req.Dest, req.Force, req.Ref
	warnf := warnTo(stderr)
	client := fetch.New(req.Token, warnf)

	partial, err := ghurl.Parse(url)
	if err != nil {
		return fmt.Errorf("parsing URL: %w", err)
	}
	src, err := partial.Resolve(
		client.CheckRef(partial.Owner, partial.Repo),
		client.ListRefs(partial.Owner, partial.Repo),
		func() (string, error) { return client.DefaultBranch(partial.Owner, partial.Repo) },
	)
	if err != nil {
		return fmt.Errorf("resolving URL: %w", err)
	}

	// --ref takes precedence over the branch embedded in the URL. The URL's ref
	// is still resolved above so the folder path can be split correctly.
	if refOverride != "" {
		sha, ok, err := client.CheckRef(src.Owner, src.Repo)(refOverride)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s", ErrRefNotFound, refOverride)
		}
		src = ghurl.Build(src.Owner, src.Repo, refOverride, src.Path, sha)
	}

	if dest == "" {
		name := src.Repo // whole-repo download
		if src.Path != "" {
			name = filepath.Base(src.Path)
		}
		dest = "./" + name
	}

	// Decide how to write the destination (in-place / replace / refuse) using the
	// pure policy in internal/dest; only the existence check touches the disk.
	abs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("resolving destination: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}
	_, statErr := os.Lstat(dest)
	exists := statErr == nil

	plan := destpolicy.Resolve(abs, cwd, exists, force)
	mode := fetch.Replace
	switch plan.Action {
	case destpolicy.Refuse:
		return errors.New(plan.Reason) //nolint:err113 // the destination policy owns this message; callers only print it
	case destpolicy.WriteInPlace:
		mode = fetch.Merge
	}

	folderLabel := src.Path
	if folderLabel == "" {
		folderLabel = "(whole repo)"
	}
	destLabel := dest
	if plan.Action == destpolicy.WriteInPlace {
		destLabel = dest + " (in place — existing files with the same name are overwritten)"
	}
	outln(stdout, "Downloading from GitHub...")
	outf(stdout, "  Repository: %s/%s\n", src.Owner, src.Repo)
	outf(stdout, "  Ref:        %s\n", src.Ref)
	outf(stdout, "  Folder:     %s\n", folderLabel)
	outf(stdout, "  Destination:%s\n\n", " "+destLabel)

	files, err := client.Materialize(src, dest, mode, nil, warnf)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	m := metaFrom(src)
	m.Files = files
	if err := meta.Write(dest, m); err != nil {
		return fmt.Errorf("recording the source: %w", err)
	}

	outln(stdout, "Done:")
	outln(stdout, "  "+dest)
	return nil
}

func runUpdate(token, refOverride string, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}
	// A plain "update" in a directory that is not itself a gh-get folder but
	// holds a list file updates the list instead.
	if refOverride == "" {
		if _, err := meta.Read(cwd); errors.Is(err, fs.ErrNotExist) {
			if _, err := os.Stat(manifest.DefaultFile); err == nil {
				return runList(listRequest{Path: manifest.DefaultFile, Token: token, Refresh: true}, stdout, stderr)
			}
		}
	}
	warnf := warnTo(stderr)
	client := fetch.New(token, warnf)
	_, err = updateAt(client, cwd, refOverride, stdout, warnf)
	return err
}

type updateStatus int

const (
	updateNotManaged updateStatus = iota
	updateCurrent
	updateChanged
)

// updateAt updates one gh-get installation, printing its progress to stdout and
// its non-fatal notices to warnf. Callers that present compact aggregate output
// using the returned status pass io.Discard as stdout.
func updateAt(client *fetch.Client, dir, refOverride string, stdout io.Writer, warnf func(string)) (updateStatus, error) {
	m, err := meta.Read(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Not a gh-get folder; nothing to do (parity with original).
			return updateNotManaged, nil
		}
		return updateNotManaged, fmt.Errorf("reading the source marker: %w", err)
	}
	if m.SourceURL == "" {
		return updateNotManaged, nil
	}
	if m.Owner == "" || m.Repo == "" || m.Branch == "" {
		return updateNotManaged, ErrInvalidMarker
	}

	// --ref switches the folder to a different branch/tag; otherwise stay on the
	// ref recorded in the marker.
	ref := m.Branch
	if refOverride != "" {
		ref = refOverride
	}

	sha, ok, err := client.CheckRef(m.Owner, m.Repo)(ref)
	if err != nil {
		return updateNotManaged, fmt.Errorf("checking ref: %w", err)
	}
	if !ok {
		return updateNotManaged, fmt.Errorf("%w: %s", ErrRefGone, ref)
	}
	// Short-circuit only when staying on the same ref at the same commit.
	if ref == m.Branch && sha == m.Commit && m.Commit != "" {
		outln(stdout, "Already up to date.")
		return updateCurrent, nil
	}

	src := ghurl.Build(m.Owner, m.Repo, ref, m.FolderPath, sha)

	outln(stdout, "Updating current folder via gh-get...")
	outf(stdout, "  Source: %s\n", m.SourceURL)
	outf(stdout, "  Dest:   %s\n\n", dir)

	// Materialize downloads to a temp area first, so a failure never destroys
	// local content; Sync removes only files the previous download wrote, so
	// unrelated content sharing the folder is kept.
	files, err := client.Materialize(src, dir, fetch.Sync, m.Files, warnf)
	if err != nil {
		return updateNotManaged, fmt.Errorf("updating: %w", err)
	}
	nm := metaFrom(src)
	nm.Files = files
	if err := meta.Write(dir, nm); err != nil {
		return updateNotManaged, fmt.Errorf("recording the source: %w", err)
	}

	outln(stdout, "Updated:")
	outln(stdout, "  "+dir)
	return updateChanged, nil
}

type pathError struct {
	Path string
	Err  error
}

type recursiveDiscovery struct {
	Targets []string
	Skipped []string
	Errors  []pathError
}

// discoverRecursive finds managed folders and excludes any managed parent that
// contains another installation. WalkDir does not follow directory symlinks.
func discoverRecursive(root string) recursiveDiscovery {
	var found []string
	var scanErrors []pathError
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			scanErrors = append(scanErrors, pathError{Path: path, Err: err})
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == meta.FileName && !entry.IsDir() {
			found = append(found, filepath.Dir(path))
		}
		return nil
	})
	if walkErr != nil {
		scanErrors = append(scanErrors, pathError{Path: root, Err: walkErr})
	}

	sort.Slice(found, func(i, j int) bool {
		di, dj := pathDepth(root, found[i]), pathDepth(root, found[j])
		if di != dj {
			return di > dj
		}
		return found[i] < found[j]
	})

	result := recursiveDiscovery{Errors: scanErrors}
	for _, candidate := range found {
		parent := false
		for _, other := range found {
			if candidate != other && isDescendant(candidate, other) {
				parent = true
				break
			}
		}
		if parent {
			result.Skipped = append(result.Skipped, candidate)
		} else {
			result.Targets = append(result.Targets, candidate)
		}
	}
	return result
}

func pathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(filepath.Clean(rel), string(os.PathSeparator)) + 1
}

func isDescendant(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

type recursiveSummary struct {
	Updated int
	Current int
	Skipped int
	Failed  int
}

type updateAttempt struct {
	Path   string
	Status updateStatus
	Err    error
}

func updateAll(paths []string, updater func(string) (updateStatus, error)) []updateAttempt {
	attempts := make([]updateAttempt, 0, len(paths))
	for _, path := range paths {
		status, err := updater(path)
		attempts = append(attempts, updateAttempt{Path: path, Status: status, Err: err})
	}
	return attempts
}

func runRecursiveUpdate(token string, stdout, stderr io.Writer) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}

	outln(stdout, "Searching for gh-get installations under:")
	outln(stdout, "  "+root)
	discovery := discoverRecursive(root)
	if len(discovery.Targets) == 0 && len(discovery.Skipped) == 0 && len(discovery.Errors) == 0 {
		outln(stdout, "No gh-get installations found.")
		return nil
	}

	summary := recursiveSummary{Skipped: len(discovery.Skipped), Failed: len(discovery.Errors)}
	for _, path := range discovery.Skipped {
		outf(stdout, "[skipped] %s (contains a nested gh-get installation)\n", displayPath(root, path))
	}
	for _, scanErr := range discovery.Errors {
		outf(stdout, "[failed]  %s: %v\n", displayPath(root, scanErr.Path), scanErr.Err)
	}

	warnf := warnTo(stderr)
	client := fetch.New(token, warnf)
	attempts := updateAll(discovery.Targets, func(path string) (updateStatus, error) {
		return updateAt(client, path, "", io.Discard, warnf)
	})
	for _, attempt := range attempts {
		switch {
		case attempt.Err != nil:
			summary.Failed++
			outf(stdout, "[failed]  %s: %v\n", displayPath(root, attempt.Path), attempt.Err)
		case attempt.Status == updateChanged:
			summary.Updated++
			outf(stdout, "[updated] %s\n", displayPath(root, attempt.Path))
		case attempt.Status == updateCurrent:
			summary.Current++
			outf(stdout, "[current] %s\n", displayPath(root, attempt.Path))
		default:
			summary.Failed++
			outf(stdout, "[failed]  %s: invalid or missing %s\n", displayPath(root, attempt.Path), meta.FileName)
		}
	}

	outln(stdout, "\nSummary:")
	outf(stdout, "  Updated:            %d\n", summary.Updated)
	outf(stdout, "  Already up to date: %d\n", summary.Current)
	outf(stdout, "  Skipped:            %d\n", summary.Skipped)
	outf(stdout, "  Failed:             %d\n", summary.Failed)
	if summary.Failed > 0 {
		return fmt.Errorf("%d %w", summary.Failed, ErrRecursiveFailed)
	}
	return nil
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return "."
	}
	return rel
}

func metaFrom(src ghurl.Source) meta.Meta {
	return meta.Meta{
		SourceURL:  src.URL,
		Owner:      src.Owner,
		Repo:       src.Repo,
		Branch:     src.Ref,
		FolderPath: src.Path,
		Commit:     src.Commit,
	}
}

func resolveToken(flag string) string {
	if flag != "" {
		return flag
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return tokenFromGHCLI()
}

// tokenFromGHCLI returns the token stored by an authenticated GitHub CLI
// (`gh auth token`), or "" if gh is absent, not logged in, or slow to respond.
// Any failure is silent so callers fall back to anonymous access.
func tokenFromGHCLI() string {
	if _, err := exec.LookPath("gh"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// warnTo returns a notice sink that prints each non-fatal notice to w.
func warnTo(w io.Writer) func(string) {
	return func(msg string) {
		outln(w, "warning: "+msg)
	}
}

// warn prints a non-fatal notice to the process's stderr. It is for leaf
// cleanup helpers (closing or removing a file) that sit below any writer.
func warn(msg string) {
	warnTo(os.Stderr)(msg)
}

// outf writes formatted terminal output to w, as fmt.Fprintf.
func outf(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, format, a...) //nolint:errcheck // terminal output, like printing to os.Stdout: a failed write to stdout/stderr has no one left to report to
}

// outln writes one line of terminal output to w, as fmt.Fprintln.
func outln(w io.Writer, a ...any) {
	fmt.Fprintln(w, a...) //nolint:errcheck // terminal output, like printing to os.Stdout: a failed write to stdout/stderr has no one left to report to
}
