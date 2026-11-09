package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

// listAction is what syncing one list entry does to its destination.
type listAction int

const (
	listInstall listAction = iota // destination absent (or --force): download
	listUpdate                    // same source, different ref/commit: re-download
	listCurrent                   // nothing to do
)

// Errors of syncing one list entry; each prints exactly its text.
var (
	ErrUseForce         = errors.New("use --force to replace it")
	ErrEntriesFailed    = errors.New("failed")
	ErrDestContainsList = errors.New("destination contains the list file's directory")
	ErrDestContainsCwd  = errors.New("destination contains the current directory")
	ErrDestOverlap      = errors.New("destination overlaps another entry")
)

// destState describes what is on disk at an entry's destination.
type destState struct {
	Exists  bool
	Managed bool      // has a readable .gh-get-source
	Meta    meta.Meta // valid when Managed
}

// planListEntry decides how to sync src into a destination in state st. With
// refresh (update), an installation of the same source is moved to the entry's
// ref/commit; without it (download), an existing installation is left alone.
// A destination holding anything else is only overwritten with --force.
func planListEntry(st destState, src ghurl.Source, refresh, force bool) (listAction, error) {
	switch {
	case !st.Exists:
		return listInstall, nil
	case st.Managed && sameSource(st.Meta, src):
		if !refresh || (st.Meta.Branch == src.Ref && st.Meta.Commit == src.Commit && src.Commit != "") {
			return listCurrent, nil
		}
		return listUpdate, nil
	case force:
		return listInstall, nil
	case st.Managed:
		return 0, fmt.Errorf("destination holds a different gh-get download (%s); %w", st.Meta.SourceURL, ErrUseForce)
	default:
		return 0, fmt.Errorf("destination exists and was not downloaded by gh-get; %w", ErrUseForce)
	}
}

// sameSource reports whether a marker records the same repo folder as src,
// regardless of ref.
func sameSource(m meta.Meta, src ghurl.Source) bool {
	return strings.EqualFold(m.Owner, src.Owner) &&
		strings.EqualFold(m.Repo, src.Repo) &&
		strings.Trim(m.FolderPath, "/") == src.Path
}

// containsOrEqual reports whether wiping parent would also affect child.
func containsOrEqual(parent, child string) bool {
	return eqPath(parent, child) || isDescendant(parent, child)
}

// listRequest is one run over a list file asked for on the command line.
type listRequest struct {
	Path    string // the list file
	Token   string
	Refresh bool // update existing entries too ("update -F"), not only download missing ones
	Force   bool
}

// runList downloads (Refresh false) or updates (Refresh true) every entry of
// the list file at req.Path, printing progress to stdout and notices to stderr.
// Relative destinations resolve against the list file's directory. Entries are
// processed sequentially and failures do not stop the run; the command returns
// non-zero if any entry failed.
func runList(req listRequest, stdout, stderr io.Writer) error {
	listPath, token, refresh, force := req.Path, req.Token, req.Refresh, req.Force
	absList, err := filepath.Abs(listPath)
	if err != nil {
		return fmt.Errorf("resolving the list file: %w", err)
	}
	entries, err := manifest.Load(absList)
	if err != nil {
		return fmt.Errorf("loading the list file: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}
	baseDir := filepath.Dir(absList)

	verb := "Downloading"
	if refresh {
		verb = "Updating"
	}
	outf(stdout, "%s %d entr%s from %s\n", verb, len(entries), plural(len(entries), "y", "ies"), displayPath(cwd, absList))
	if len(entries) == 0 {
		return nil
	}

	client := fetch.New(token, warnTo(stderr))
	var claimed []string
	var installed, updated, current, failed int
	for _, e := range entries {
		label := e.URL
		dest, action, err := syncListEntry(client, e, baseDir, cwd, claimed, refresh, force)
		if dest != "" {
			label = displayPath(cwd, dest)
			claimed = append(claimed, dest)
		}
		switch {
		case err != nil:
			failed++
			outf(stdout, "[failed]    line %d: %s: %v\n", e.Line, label, err)
		case action == listInstall:
			installed++
			outf(stdout, "[installed] %s\n", label)
		case action == listUpdate:
			updated++
			outf(stdout, "[updated]   %s\n", label)
		case refresh:
			current++
			outf(stdout, "[current]   %s\n", label)
		default:
			current++
			outf(stdout, "[present]   %s\n", label)
		}
	}

	outln(stdout, "\nSummary:")
	outf(stdout, "  Installed:          %d\n", installed)
	if refresh {
		outf(stdout, "  Updated:            %d\n", updated)
		outf(stdout, "  Already up to date: %d\n", current)
	} else {
		outf(stdout, "  Already present:    %d\n", current)
	}
	outf(stdout, "  Failed:             %d\n", failed)
	if failed > 0 {
		return fmt.Errorf("%d list entr%s %w", failed, plural(failed, "y", "ies"), ErrEntriesFailed)
	}
	return nil
}

// syncListEntry resolves one entry and brings its destination in line with it.
// It returns the absolute destination as soon as it is known so the caller can
// label output and detect overlapping entries.
//
// An entry already installed at its destination is recognized offline, so a
// download run costs no request for it and an update run only re-checks the
// installed ref's commit.
func syncListEntry(client *fetch.Client, e manifest.Entry, baseDir, cwd string, claimed []string, refresh, force bool) (string, listAction, error) {
	partial, err := ghurl.Parse(e.URL)
	if err != nil {
		return "", 0, fmt.Errorf("parsing URL: %w", err)
	}

	src, dest, known := installedSource(e, baseDir, !refresh)
	switch {
	case !known:
		src, err = partial.Resolve(
			client.CheckRef(partial.Owner, partial.Repo),
			client.ListRefs(partial.Owner, partial.Repo),
			func() (string, error) { return client.DefaultBranch(partial.Owner, partial.Repo) },
		)
		if err != nil {
			return "", 0, fmt.Errorf("resolving URL: %w", err)
		}
		dest = entryDest(e, baseDir, src.Repo, src.Path)
	case refresh:
		sha, ok, refErr := client.CheckRef(src.Owner, src.Repo)(src.Ref)
		if refErr != nil {
			return dest, 0, fmt.Errorf("checking ref: %w", refErr)
		}
		if !ok {
			return dest, 0, fmt.Errorf("%w: %s", ErrRefGone, src.Ref)
		}
		src = ghurl.Build(src.Owner, src.Repo, src.Ref, src.Path, sha)
	}

	if containsOrEqual(dest, baseDir) {
		return dest, 0, ErrDestContainsList
	}
	if containsOrEqual(dest, cwd) {
		return dest, 0, ErrDestContainsCwd
	}
	for _, other := range claimed {
		if containsOrEqual(dest, other) || containsOrEqual(other, dest) {
			return dest, 0, fmt.Errorf("%w (%s)", ErrDestOverlap, displayPath(cwd, other))
		}
	}

	st, err := readDestState(dest)
	if err != nil {
		return dest, 0, err
	}
	action, err := planListEntry(st, src, refresh, force)
	if err != nil || action == listCurrent {
		return dest, action, err
	}
	// Materialize stages the download first, so a failure leaves dest intact.
	// An existing installation is synced (only its tracked files are removed);
	// anything else being overwritten was approved via --force.
	mode := fetch.Replace
	if st.Managed {
		mode = fetch.Sync
	}
	files, err := client.Materialize(src, dest, mode, st.Meta.Files, warn)
	if err != nil {
		return dest, 0, fmt.Errorf("downloading: %w", err)
	}
	m := metaFrom(src)
	m.Files = files
	if err := meta.Write(dest, m); err != nil {
		return dest, 0, fmt.Errorf("recording the source: %w", err)
	}
	return dest, action, nil
}

// entryDest is the absolute destination of entry e for a source folder path in
// repo: the explicit destination, else the folder's basename (repo name for a
// whole-repo download), relative to baseDir.
func entryDest(e manifest.Entry, baseDir, repo, folder string) string {
	destRel := e.Dest
	if destRel == "" {
		destRel = repo
		if folder != "" {
			destRel = filepath.Base(folder)
		}
	}
	dest := filepath.Clean(destRel)
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(baseDir, dest)
	}
	return dest
}

// installedSource recognizes, without network, an entry whose destination
// already holds the installation its URL describes. It returns that source at
// the installed commit and its destination. A bare repo URL tracks the default
// branch, which only the API knows, so it is recognized only with allowBare
// (download mode, where the ref does not matter).
func installedSource(e manifest.Entry, baseDir string, allowBare bool) (ghurl.Source, string, bool) {
	segs := urlSegments(e.URL)
	if len(segs) < 2 || (len(segs) == 2 && !allowBare) {
		return ghurl.Source{}, "", false
	}
	for _, dest := range entryCandidates(e, baseDir) {
		st, err := readDestState(dest)
		if err != nil || !st.Managed || st.Meta.Branch == "" || !entryDescribes(e.URL, st.Meta) {
			continue
		}
		m := st.Meta
		folder := strings.Trim(m.FolderPath, "/")
		if !eqPath(dest, entryDest(e, baseDir, m.Repo, folder)) {
			continue // e.g. a folder named after the repo holding a sub-folder install
		}
		return ghurl.Build(m.Owner, m.Repo, m.Branch, folder, m.Commit), dest, true
	}
	return ghurl.Source{}, "", false
}

func readDestState(dest string) (destState, error) {
	info, err := os.Lstat(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return destState{}, nil
	}
	if err != nil {
		return destState{}, fmt.Errorf("inspecting %s: %w", dest, err)
	}
	if !info.IsDir() {
		return destState{Exists: true}, nil
	}
	m, err := meta.Read(dest)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && m.SourceURL == "") {
		return destState{Exists: true}, nil
	}
	if err != nil {
		return destState{}, fmt.Errorf("reading the source marker: %w", err)
	}
	return destState{Exists: true, Managed: true, Meta: m}, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
