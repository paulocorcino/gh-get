package main

import (
	"fmt"
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
		return 0, fmt.Errorf("destination holds a different gh-get download (%s); use --force to replace it", st.Meta.SourceURL)
	default:
		return 0, fmt.Errorf("destination exists and was not downloaded by gh-get; use --force to replace it")
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

// runList downloads (refresh=false) or updates (refresh=true) every entry of
// the list file at listPath. Relative destinations resolve against the list
// file's directory. Entries are processed sequentially and failures do not stop
// the run; the command returns non-zero if any entry failed.
func runList(listPath, token string, refresh, force bool) error {
	absList, err := filepath.Abs(listPath)
	if err != nil {
		return err
	}
	entries, err := manifest.Load(absList)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(absList)

	verb := "Downloading"
	if refresh {
		verb = "Updating"
	}
	fmt.Printf("%s %d entr%s from %s\n", verb, len(entries), plural(len(entries), "y", "ies"), displayPath(cwd, absList))
	if len(entries) == 0 {
		return nil
	}

	client := fetch.New(token, warn)
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
			fmt.Printf("[failed]    line %d: %s: %v\n", e.Line, label, err)
		case action == listInstall:
			installed++
			fmt.Printf("[installed] %s\n", label)
		case action == listUpdate:
			updated++
			fmt.Printf("[updated]   %s\n", label)
		case refresh:
			current++
			fmt.Printf("[current]   %s\n", label)
		default:
			current++
			fmt.Printf("[present]   %s\n", label)
		}
	}

	fmt.Println("\nSummary:")
	fmt.Printf("  Installed:          %d\n", installed)
	if refresh {
		fmt.Printf("  Updated:            %d\n", updated)
		fmt.Printf("  Already up to date: %d\n", current)
	} else {
		fmt.Printf("  Already present:    %d\n", current)
	}
	fmt.Printf("  Failed:             %d\n", failed)
	if failed > 0 {
		return fmt.Errorf("%d list entr%s failed", failed, plural(failed, "y", "ies"))
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
		return "", 0, err
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
			return "", 0, err
		}
		dest = entryDest(e, baseDir, src.Repo, src.Path)
	case refresh:
		sha, ok, err := client.CheckRef(src.Owner, src.Repo)(src.Ref)
		if err != nil {
			return dest, 0, err
		}
		if !ok {
			return dest, 0, fmt.Errorf("ref no longer exists: %s", src.Ref)
		}
		src = ghurl.Build(src.Owner, src.Repo, src.Ref, src.Path, sha)
	}

	if containsOrEqual(dest, baseDir) {
		return dest, 0, fmt.Errorf("destination contains the list file's directory")
	}
	if containsOrEqual(dest, cwd) {
		return dest, 0, fmt.Errorf("destination contains the current directory")
	}
	for _, other := range claimed {
		if containsOrEqual(dest, other) || containsOrEqual(other, dest) {
			return dest, 0, fmt.Errorf("destination overlaps another entry (%s)", displayPath(cwd, other))
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
	if err := client.Materialize(src, dest, fetch.Replace, warn); err != nil {
		return dest, 0, err
	}
	if err := meta.Write(dest, metaFrom(src)); err != nil {
		return dest, 0, err
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
	if os.IsNotExist(err) {
		return destState{}, nil
	}
	if err != nil {
		return destState{}, err
	}
	if !info.IsDir() {
		return destState{Exists: true}, nil
	}
	m, err := meta.Read(dest)
	if os.IsNotExist(err) || (err == nil && m.SourceURL == "") {
		return destState{Exists: true}, nil
	}
	if err != nil {
		return destState{}, err
	}
	return destState{Exists: true, Managed: true, Meta: m}, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
