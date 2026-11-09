package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

// installation is a managed folder found on disk.
type installation struct {
	Dir  string // absolute
	Meta meta.Meta
}

type freezeStatus int

const (
	freezeKept    freezeStatus = iota // an entry already describes the folder
	freezeChanged                     // an entry's URL was rewritten to the installed ref
	freezeAdded                       // a new line was appended
)

type freezeResult struct {
	Inst   installation
	Status freezeStatus
}

// mergeFreeze brings a list file (its raw lines and parsed entries) in line
// with the installations found under baseDir. Existing lines, comments and
// order are preserved; an entry matching an installation keeps its line,
// with the URL rewritten when the installed source differs (e.g. after
// "update --ref"); installations without an entry are appended. Entries that
// match nothing on disk are kept, since the list may declare folders that are
// not downloaded yet.
func mergeFreeze(lines []string, entries []manifest.Entry, baseDir string, insts []installation) ([]string, []freezeResult) {
	out := append([]string(nil), lines...)
	matched := map[int]bool{} // entry index already claimed
	var results []freezeResult
	var added []installation

	for _, inst := range insts {
		idx := matchEntry(entries, matched, baseDir, inst.Dir)
		if idx < 0 {
			added = append(added, inst)
			continue
		}
		matched[idx] = true
		e := entries[idx]
		if entryDescribes(e.URL, inst.Meta) {
			results = append(results, freezeResult{inst, freezeKept})
			continue
		}
		out[e.Line-1] = strings.Replace(out[e.Line-1], e.URL, sourceURL(inst.Meta), 1)
		results = append(results, freezeResult{inst, freezeChanged})
	}

	sort.Slice(added, func(i, j int) bool { return added[i].Dir < added[j].Dir })
	for _, inst := range added {
		out = append(out, freezeLine(inst, baseDir))
		results = append(results, freezeResult{inst, freezeAdded})
	}
	return out, results
}

// matchEntry returns the index of the first entry not yet matched that puts
// its folder at dir, or -1.
func matchEntry(entries []manifest.Entry, matched map[int]bool, baseDir, dir string) int {
	for i, e := range entries {
		if !matched[i] && entryTargets(e, baseDir, dir) {
			return i
		}
	}
	return -1
}

// freezeLine is the list line that describes inst: its source URL, followed by
// its destination relative to baseDir unless that is the default one.
func freezeLine(inst installation, baseDir string) string {
	line := sourceURL(inst.Meta)
	rel, err := filepath.Rel(baseDir, inst.Dir)
	if err != nil {
		rel = inst.Dir
	}
	if rel != defaultDestName(inst.Meta) {
		line += "  " + filepath.ToSlash(rel)
	}
	return line
}

// entryTargets reports whether entry e (offline, without resolving refs) puts
// its folder at dir. With no explicit destination the default name depends on
// how the ref splits from the path, so both plausible defaults are accepted:
// the last URL segment and the repo name.
func entryTargets(e manifest.Entry, baseDir, dir string) bool {
	for _, d := range entryCandidates(e, baseDir) {
		if eqPath(d, dir) {
			return true
		}
	}
	return false
}

// entryCandidates lists the absolute destinations entry e may put its folder
// at, judged offline: the explicit destination, else the last URL segment and
// the repo name.
func entryCandidates(e manifest.Entry, baseDir string) []string {
	if e.Dest != "" {
		d := filepath.Clean(e.Dest)
		if !filepath.IsAbs(d) {
			d = filepath.Join(baseDir, d)
		}
		return []string{d}
	}
	segs := urlSegments(e.URL)
	if len(segs) < 2 {
		return nil
	}
	return []string{filepath.Join(baseDir, segs[len(segs)-1]), filepath.Join(baseDir, segs[1])}
}

// entryDescribes reports whether a list URL already points at the installed
// source. A bare repo URL (default branch) still describes a whole-repo
// installation, so freezing does not pin it to the branch it resolved to.
func entryDescribes(rawURL string, m meta.Meta) bool {
	segs := urlSegments(rawURL)
	if len(segs) == 2 {
		return m.FolderPath == "" &&
			strings.EqualFold(segs[0], m.Owner) && strings.EqualFold(segs[1], m.Repo)
	}
	if len(segs) > 2 && segs[2] == "blob" {
		segs[2] = "tree"
	}
	return strings.EqualFold("https://github.com/"+strings.Join(segs, "/"), sourceURL(m))
}

// urlSegments returns the path segments of a github.com URL (no query/fragment).
func urlSegments(raw string) []string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	i := strings.Index(raw, "github.com/")
	if i < 0 {
		return nil
	}
	return strings.Split(strings.Trim(raw[i+len("github.com/"):], "/"), "/")
}

func sourceURL(m meta.Meta) string {
	return ghurl.Build(m.Owner, m.Repo, m.Branch, strings.Trim(m.FolderPath, "/"), "").URL
}

func defaultDestName(m meta.Meta) string {
	if p := strings.Trim(m.FolderPath, "/"); p != "" {
		return filepath.Base(p)
	}
	return m.Repo
}

// ErrFoldersUnread ends a freeze that could not read some folders.
var ErrFoldersUnread = errors.New("folder(s) could not be read")

const freezeHeader = `# gh-get list: one GitHub URL per line, optionally followed by a destination
# (relative to this file). "gh-get update" re-syncs everything listed here.
`

// runFreeze scans the list file's directory for gh-get folders and creates or
// updates the list file so it describes them, printing progress to stdout.
func runFreeze(listPath string, stdout io.Writer) error {
	absList, err := filepath.Abs(listPath)
	if err != nil {
		return fmt.Errorf("resolving the list file: %w", err)
	}
	baseDir := filepath.Dir(absList)

	list, err := readFreezeList(absList, listPath)
	if err != nil {
		return err
	}

	outln(stdout, "Searching for gh-get installations under:")
	outln(stdout, "  "+baseDir)
	insts, failed := scanInstallations(baseDir, stdout)

	out, results := mergeFreeze(list.Lines, list.Entries, baseDir, insts)
	tally := reportFreeze(results, baseDir, stdout)

	if tally.Added+tally.Changed > 0 || !list.Exists {
		if err := os.WriteFile(absList, []byte(strings.Join(out, "\n")+"\n"), 0o644); err != nil { //nolint:gosec // G703: the list file path is the user's own command-line argument by design
			return fmt.Errorf("writing the list file: %w", err)
		}
		outf(stdout, "\nWrote %s\n", absList)
	} else {
		outf(stdout, "\n%s is already up to date.\n", absList)
	}
	outln(stdout, "Summary:")
	outf(stdout, "  Added:          %d\n", tally.Added)
	outf(stdout, "  Changed:        %d\n", tally.Changed)
	outf(stdout, "  Already listed: %d\n", tally.Kept)
	outf(stdout, "  Failed:         %d\n", failed)
	if failed > 0 {
		return fmt.Errorf("%d %w", failed, ErrFoldersUnread)
	}
	return nil
}

// freezeList is a list file as freeze reads it: its raw lines, its parsed
// entries, and whether it exists (a missing one starts from freezeHeader).
type freezeList struct {
	Lines   []string
	Entries []manifest.Entry
	Exists  bool
}

// readFreezeList reads the list file at absList; listPath names it in a parse
// error.
func readFreezeList(absList, listPath string) (freezeList, error) {
	data, err := os.ReadFile(absList) //nolint:gosec // G304,G703: the list file path is the user's own command-line argument by design
	if errors.Is(err, os.ErrNotExist) {
		return freezeList{Lines: strings.Split(strings.TrimSuffix(freezeHeader, "\n"), "\n")}, nil
	}
	if err != nil {
		return freezeList{}, fmt.Errorf("reading the list file: %w", err)
	}
	list := freezeList{Exists: true}
	text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if text != "" {
		list.Lines = strings.Split(text, "\n")
	}
	if list.Entries, err = manifest.Parse(strings.NewReader(string(data))); err != nil {
		return freezeList{}, fmt.Errorf("%s: %w", listPath, err)
	}
	return list, nil
}

// scanInstallations finds the gh-get folders under baseDir, printing the ones
// it skips or cannot read, and returns the installations with the count of
// failures.
func scanInstallations(baseDir string, stdout io.Writer) ([]installation, int) {
	discovery := discoverRecursive(baseDir)
	failed := len(discovery.Errors)
	for _, scanErr := range discovery.Errors {
		outf(stdout, "[failed]  %s: %v\n", displayPath(baseDir, scanErr.Path), scanErr.Err)
	}
	for _, path := range discovery.Skipped {
		outf(stdout, "[skipped] %s (contains a nested gh-get installation)\n", displayPath(baseDir, path))
	}

	var insts []installation
	for _, dir := range discovery.Targets {
		if eqPath(dir, baseDir) {
			outln(stdout, "[skipped] . (the list file's own directory cannot be a list entry)")
			continue
		}
		m, err := meta.Read(dir)
		if err == nil {
			err = checkMarker(m)
		}
		if err != nil {
			failed++
			outf(stdout, "[failed]  %s: %v\n", displayPath(baseDir, dir), err)
			continue
		}
		insts = append(insts, installation{Dir: dir, Meta: m})
	}
	return insts, failed
}

// checkMarker requires a source marker to name an owner, a repo and a branch.
func checkMarker(m meta.Meta) error {
	if m.Owner == "" || m.Repo == "" || m.Branch == "" {
		return ErrInvalidMarker
	}
	return nil
}

// freezeTally counts the list entries a freeze added, changed and kept.
type freezeTally struct {
	Added   int
	Changed int
	Kept    int
}

// reportFreeze prints one line per merge result, sorted by folder, and counts
// them.
func reportFreeze(results []freezeResult, baseDir string, stdout io.Writer) freezeTally {
	sort.Slice(results, func(i, j int) bool { return results[i].Inst.Dir < results[j].Inst.Dir })
	var tally freezeTally
	for _, r := range results {
		label := displayPath(baseDir, r.Inst.Dir)
		switch r.Status {
		case freezeAdded:
			tally.Added++
			outf(stdout, "[added]   %s\n", label)
		case freezeChanged:
			tally.Changed++
			outf(stdout, "[changed] %s -> %s\n", label, sourceURL(r.Inst.Meta))
		case freezeKept:
			tally.Kept++
			outf(stdout, "[listed]  %s\n", label)
		}
	}
	return tally
}
