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
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}
	client := fetch.New(req.Token, warnTo(stderr))
	list, err := loadList(req.Path, cwd, client)
	if err != nil {
		return err
	}

	verb := "Downloading"
	if req.Refresh {
		verb = "Updating"
	}
	outf(stdout, "%s %d entr%s from %s\n", verb, len(list.Entries), plural(len(list.Entries), "y", "ies"), list.Label)
	if len(list.Entries) == 0 {
		return nil
	}

	s := listSync{
		Client:  client,
		BaseDir: list.BaseDir,
		ListDir: list.ListDir,
		Cwd:     cwd,
		Refresh: req.Refresh,
		Force:   req.Force,
	}
	var claimed []string
	tally := listTally{Refresh: req.Refresh}
	for _, e := range list.Entries {
		label := e.URL
		dest, action, err := syncListEntry(s, e, claimed)
		if dest != "" {
			label = displayPath(cwd, dest)
			claimed = append(claimed, dest)
		}
		tally.record(stdout, e.Line, label, action, err)
	}
	return tally.summarize(stdout)
}

// listTally counts the outcomes of a run over a list file.
type listTally struct {
	Refresh   bool // an update run ("update -F"): an entry left alone is current, not present
	Installed int
	Updated   int
	Current   int
	Failed    int
}

// record counts the outcome of syncing the entry at line and prints its line
// of progress.
func (t *listTally) record(stdout io.Writer, line int, label string, action listAction, err error) {
	switch {
	case err != nil:
		t.Failed++
		outf(stdout, "[failed]    line %d: %s: %v\n", line, label, err)
	case action == listInstall:
		t.Installed++
		outf(stdout, "[installed] %s\n", label)
	case action == listUpdate:
		t.Updated++
		outf(stdout, "[updated]   %s\n", label)
	case t.Refresh:
		t.Current++
		outf(stdout, "[current]   %s\n", label)
	default:
		t.Current++
		outf(stdout, "[present]   %s\n", label)
	}
}

// summarize prints the summary of the run and returns an error when any entry
// failed.
func (t *listTally) summarize(stdout io.Writer) error {
	outln(stdout, "\nSummary:")
	outf(stdout, "  Installed:          %d\n", t.Installed)
	if t.Refresh {
		outf(stdout, "  Updated:            %d\n", t.Updated)
		outf(stdout, "  Already up to date: %d\n", t.Current)
	} else {
		outf(stdout, "  Already present:    %d\n", t.Current)
	}
	outf(stdout, "  Failed:             %d\n", t.Failed)
	if t.Failed > 0 {
		return fmt.Errorf("%d list entr%s %w", t.Failed, plural(t.Failed, "y", "ies"), ErrEntriesFailed)
	}
	return nil
}

// listSync is what syncing every entry of one list file shares.
type listSync struct {
	Client  *fetch.Client
	BaseDir string // relative destinations resolve against it
	ListDir string // the list file's directory, which no destination may contain; "" for a list read from GitHub
	Cwd     string
	Refresh bool // update existing entries too, not only download missing ones
	Force   bool
}

// syncListEntry resolves one entry and brings its destination in line with it.
// It returns the absolute destination as soon as it is known so the caller can
// label output and detect overlapping entries (claimed holds the destinations
// of the entries before it).
//
// An entry already installed at its destination is recognized offline, so a
// download run costs no request for it and an update run only re-checks the
// installed ref's commit.
func syncListEntry(s listSync, e manifest.Entry, claimed []string) (string, listAction, error) {
	src, dest, err := resolveListEntry(s, e)
	if err != nil {
		return dest, 0, err
	}
	if err = checkListDest(dest, s.ListDir, s.Cwd, claimed); err != nil {
		return dest, 0, err
	}

	st, err := readDestState(dest)
	if err != nil {
		return dest, 0, err
	}
	action, err := planListEntry(st, src, s.Refresh, s.Force)
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
	files, err := s.Client.Materialize(src, dest, mode, st.Meta.Files, warn)
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

// resolveListEntry returns the source entry e describes and its absolute
// destination. An entry already installed is recognized offline; with refresh
// its installed ref is re-checked for the current commit. The destination is
// returned with an error once it is known.
func resolveListEntry(s listSync, e manifest.Entry) (ghurl.Source, string, error) {
	src, dest, known := installedSource(e, s.BaseDir, !s.Refresh)
	switch {
	case !known:
		var err error
		src, err = resolveDownload(s.Client, e.URL, "")
		if err != nil {
			return ghurl.Source{}, "", err
		}
		dest = entryDest(e, s.BaseDir, src.Repo, src.Path)
	case s.Refresh:
		sha, ok, refErr := s.Client.CheckRef(src.Owner, src.Repo)(src.Ref)
		if refErr != nil {
			return ghurl.Source{}, dest, fmt.Errorf("checking ref: %w", refErr)
		}
		if !ok {
			return ghurl.Source{}, dest, fmt.Errorf("%w: %s", ErrRefGone, src.Ref)
		}
		src = ghurl.Build(src.Owner, src.Repo, src.Ref, src.Path, sha)
	}
	return src, dest, nil
}

// checkListDest refuses a destination that would wipe the list file's
// directory, the current directory, or the destination of another entry.
func checkListDest(dest, listDir, cwd string, claimed []string) error {
	if listDir != "" && containsOrEqual(dest, listDir) {
		return ErrDestContainsList
	}
	if containsOrEqual(dest, cwd) {
		return ErrDestContainsCwd
	}
	for _, other := range claimed {
		if containsOrEqual(dest, other) || containsOrEqual(other, dest) {
			return fmt.Errorf("%w (%s)", ErrDestOverlap, displayPath(cwd, other))
		}
	}
	return nil
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
