package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

func runUpdate(token, refOverride string, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}
	// A plain "update" in a directory that is not itself a gh-get folder but
	// holds a list file updates the list instead.
	if refOverride == "" {
		if _, readErr := meta.Read(cwd); errors.Is(readErr, fs.ErrNotExist) {
			if _, statErr := os.Stat(manifest.DefaultFile); statErr == nil {
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
	m, managed, err := readUpdateMarker(dir)
	if err != nil || !managed {
		return updateNotManaged, err
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

// readUpdateMarker reads the source marker of dir and reports whether dir is a
// gh-get folder at all. A folder without a marker, or with an empty source URL,
// is not managed and is no error (parity with the original); a marker without
// owner, repo or branch is.
func readUpdateMarker(dir string) (meta.Meta, bool, error) {
	m, err := meta.Read(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return meta.Meta{}, false, nil
	}
	if err != nil {
		return meta.Meta{}, false, fmt.Errorf("reading the source marker: %w", err)
	}
	if m.SourceURL == "" {
		return meta.Meta{}, false, nil
	}
	if err := checkMarker(m); err != nil {
		return meta.Meta{}, false, err
	}
	return m, true, nil
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
	found, scanErrors := findMarkers(root)
	sortDeepestFirst(root, found)

	result := recursiveDiscovery{Errors: scanErrors}
	for _, candidate := range found {
		if containsAnother(candidate, found) {
			result.Skipped = append(result.Skipped, candidate)
		} else {
			result.Targets = append(result.Targets, candidate)
		}
	}
	return result
}

// findMarkers walks root and returns every directory holding a source marker,
// with the errors met on the way; an unreadable directory is skipped.
func findMarkers(root string) ([]string, []pathError) {
	var found []string
	var scanErrors []pathError
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error { //nolint:gosec // G703: root is the directory the user named to scan, by design
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
	return found, scanErrors
}

// sortDeepestFirst orders dirs by depth below root, deepest first, then by
// path.
func sortDeepestFirst(root string, dirs []string) {
	sort.Slice(dirs, func(i, j int) bool {
		di, dj := pathDepth(root, dirs[i]), pathDepth(root, dirs[j])
		if di != dj {
			return di > dj
		}
		return dirs[i] < dirs[j]
	})
}

// containsAnother reports whether one of dirs other than candidate lies below
// candidate.
func containsAnother(candidate string, dirs []string) bool {
	for _, other := range dirs {
		if candidate != other && isDescendant(candidate, other) {
			return true
		}
	}
	return false
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
