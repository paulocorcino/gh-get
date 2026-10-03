package main

import (
	"errors"
	"fmt"
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
		idx := -1
		for i, e := range entries {
			if !matched[i] && entryTargets(e, baseDir, inst.Dir) {
				idx = i
				break
			}
		}
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
		line := sourceURL(inst.Meta)
		rel, err := filepath.Rel(baseDir, inst.Dir)
		if err != nil {
			rel = inst.Dir
		}
		if rel != defaultDestName(inst.Meta) {
			line += "  " + filepath.ToSlash(rel)
		}
		out = append(out, line)
		results = append(results, freezeResult{inst, freezeAdded})
	}
	return out, results
}

// entryTargets reports whether entry e (offline, without resolving refs) puts
// its folder at dir. With no explicit destination the default name depends on
// how the ref splits from the path, so both plausible defaults are accepted:
// the last URL segment and the repo name.
func entryTargets(e manifest.Entry, baseDir, dir string) bool {
	if e.Dest != "" {
		d := filepath.Clean(e.Dest)
		if !filepath.IsAbs(d) {
			d = filepath.Join(baseDir, d)
		}
		return eqPath(d, dir)
	}
	segs := urlSegments(e.URL)
	if len(segs) < 2 {
		return false
	}
	return eqPath(filepath.Join(baseDir, segs[len(segs)-1]), dir) ||
		eqPath(filepath.Join(baseDir, segs[1]), dir)
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

const freezeHeader = `# gh-get list: one GitHub URL per line, optionally followed by a destination
# (relative to this file). "gh-get update" re-syncs everything listed here.
`

// runFreeze scans the list file's directory for gh-get folders and creates or
// updates the list file so it describes them.
func runFreeze(listPath string) error {
	absList, err := filepath.Abs(listPath)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(absList)

	var lines []string
	var entries []manifest.Entry
	data, err := os.ReadFile(absList)
	exists := err == nil
	switch {
	case errors.Is(err, os.ErrNotExist):
		lines = strings.Split(strings.TrimSuffix(freezeHeader, "\n"), "\n")
	case err != nil:
		return err
	default:
		text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if text != "" {
			lines = strings.Split(text, "\n")
		}
		if entries, err = manifest.Parse(strings.NewReader(string(data))); err != nil {
			return fmt.Errorf("%s: %w", listPath, err)
		}
	}

	fmt.Println("Searching for gh-get installations under:")
	fmt.Println("  " + baseDir)
	discovery := discoverRecursive(baseDir)
	failed := len(discovery.Errors)
	for _, scanErr := range discovery.Errors {
		fmt.Printf("[failed]  %s: %v\n", displayPath(baseDir, scanErr.Path), scanErr.Err)
	}
	for _, path := range discovery.Skipped {
		fmt.Printf("[skipped] %s (contains a nested gh-get installation)\n", displayPath(baseDir, path))
	}

	var insts []installation
	for _, dir := range discovery.Targets {
		if eqPath(dir, baseDir) {
			fmt.Println("[skipped] . (the list file's own directory cannot be a list entry)")
			continue
		}
		m, err := meta.Read(dir)
		if err == nil && (m.Owner == "" || m.Repo == "" || m.Branch == "") {
			err = fmt.Errorf("invalid %s: owner, repo, and branch are required", meta.FileName)
		}
		if err != nil {
			failed++
			fmt.Printf("[failed]  %s: %v\n", displayPath(baseDir, dir), err)
			continue
		}
		insts = append(insts, installation{Dir: dir, Meta: m})
	}

	out, results := mergeFreeze(lines, entries, baseDir, insts)
	sort.Slice(results, func(i, j int) bool { return results[i].Inst.Dir < results[j].Inst.Dir })
	var kept, changed, added int
	for _, r := range results {
		label := displayPath(baseDir, r.Inst.Dir)
		switch r.Status {
		case freezeAdded:
			added++
			fmt.Printf("[added]   %s\n", label)
		case freezeChanged:
			changed++
			fmt.Printf("[changed] %s -> %s\n", label, sourceURL(r.Inst.Meta))
		default:
			kept++
			fmt.Printf("[listed]  %s\n", label)
		}
	}

	if added+changed > 0 || !exists {
		if err := os.WriteFile(absList, []byte(strings.Join(out, "\n")+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nWrote %s\n", absList)
	} else {
		fmt.Printf("\n%s is already up to date.\n", absList)
	}
	fmt.Println("Summary:")
	fmt.Printf("  Added:          %d\n", added)
	fmt.Printf("  Changed:        %d\n", changed)
	fmt.Printf("  Already listed: %d\n", kept)
	fmt.Printf("  Failed:         %d\n", failed)
	if failed > 0 {
		return fmt.Errorf("%d folder(s) could not be read", failed)
	}
	return nil
}
