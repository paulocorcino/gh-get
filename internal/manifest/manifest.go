// Package manifest parses a gh-get list file (default gh-get.txt): a plain-text,
// requirements.txt-style list of GitHub URLs to download, each optionally
// followed by a destination.
//
// Format, one entry per line:
//
//	# comment
//	https://github.com/OWNER/REPO/tree/REF/PATH            (destination: ./PATH basename)
//	https://github.com/OWNER/REPO/tree/REF/PATH  my/dest   (explicit destination)
//
// Blank lines and lines starting with "#" are ignored; a "#" preceded by
// whitespace starts an inline comment. Everything after the URL is the
// destination, so it may contain spaces.
package manifest

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

// DefaultFile is the list file "gh-get update" picks up from the current
// directory when that directory is not itself a gh-get folder.
const DefaultFile = "gh-get.txt"

// Entry is one non-comment line of a list file.
type Entry struct {
	Line int    // 1-based line number, for messages
	URL  string // GitHub URL as written
	Dest string // destination as written; "" means the default name
}

// Load reads and parses the list file at path.
func Load(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return entries, nil
}

// Parse reads entries from r. Every URL is checked offline with ghurl.Parse so
// a malformed line fails the whole list before anything is downloaded.
func Parse(r io.Reader) ([]Entry, error) {
	var entries []Entry
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := stripComment(sc.Text())
		if line == "" {
			continue
		}
		url, dest := line, ""
		if i := strings.IndexAny(line, " \t"); i >= 0 {
			url, dest = line[:i], strings.TrimSpace(line[i:])
		}
		if _, err := ghurl.Parse(url); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		entries = append(entries, Entry{Line: n, URL: url, Dest: dest})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// stripComment drops a full-line or whitespace-preceded "#" comment and trims.
func stripComment(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\xef\xbb\xbf"))
	if strings.HasPrefix(line, "#") {
		return ""
	}
	for i := 1; i < len(line); i++ {
		if line[i] == '#' && (line[i-1] == ' ' || line[i-1] == '\t') {
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}
