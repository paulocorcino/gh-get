// Package meta reads and writes the .gh-get-source marker file that records a
// downloaded folder's origin, enabling `gh-get update`.
package meta

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is the hidden marker written inside every downloaded folder.
const FileName = ".gh-get-source"

// Meta is the content of a .gh-get-source file.
type Meta struct {
	SourceURL  string
	Owner      string
	Repo       string
	Branch     string // the ref (branch or tag)
	FolderPath string
	Commit     string
	UpdatedAt  string
	// Files lists, slash-separated and relative to the folder, every file
	// gh-get wrote. update removes only these, so foreign content sharing the
	// folder survives. Nil for markers written before tracking existed.
	Files []string
}

// Read loads the marker from dir. It returns os.ErrNotExist (wrapped) when the
// file is absent so callers can distinguish "not a gh-get folder".
func Read(dir string) (Meta, error) {
	f, err := os.Open(filepath.Join(dir, FileName))
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	kv := map[string]string{}
	var files []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		if line[:eq] == "file" {
			files = append(files, line[eq+1:])
			continue
		}
		kv[line[:eq]] = line[eq+1:]
	}
	if err := sc.Err(); err != nil {
		return Meta{}, err
	}

	return Meta{
		SourceURL:  kv["source_url"],
		Owner:      kv["owner"],
		Repo:       kv["repo"],
		Branch:     kv["branch"],
		FolderPath: kv["folder_path"],
		Commit:     kv["commit"],
		UpdatedAt:  kv["updated_at"],
		Files:      files,
	}, nil
}

// Write persists the marker into dir, stamping updated_at with the current time.
func Write(dir string, m Meta) error {
	body := fmt.Sprintf(
		"source_url=%s\nowner=%s\nrepo=%s\nbranch=%s\nfolder_path=%s\ncommit=%s\nupdated_at=%s\n",
		m.SourceURL, m.Owner, m.Repo, m.Branch, m.FolderPath, m.Commit,
		time.Now().Format(time.RFC3339),
	)
	var b strings.Builder
	b.WriteString(body)
	for _, f := range m.Files {
		b.WriteString("file=" + f + "\n")
	}
	return os.WriteFile(filepath.Join(dir, FileName), []byte(b.String()), 0o644)
}
