package main

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/manifest"
)

// ErrListNotFile is a list URL that names a repository or folder, not a file.
var ErrListNotFile = errors.New("the list URL must point at a file in the repository")

// loadedList is a list file read from disk or from a GitHub repository.
type loadedList struct {
	Entries []manifest.Entry
	BaseDir string // relative destinations resolve against it
	ListDir string // the list file's directory; "" for a list read from GitHub
	Label   string // how output names the list
}

// isRemoteList reports whether the list argument is a URL, not a local path.
func isRemoteList(arg string) bool {
	return strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://")
}

// loadList reads the list file named by arg: a GitHub file URL is read into
// memory (nothing is written to disk) and resolves relative destinations
// against cwd; anything else is a local path and resolves them against its own
// directory.
func loadList(arg, cwd string, client *fetch.Client) (loadedList, error) {
	if isRemoteList(arg) {
		return loadRemoteList(arg, cwd, client)
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return loadedList{}, fmt.Errorf("resolving the list file: %w", err)
	}
	entries, err := manifest.Load(abs)
	if err != nil {
		return loadedList{}, fmt.Errorf("loading the list file: %w", err)
	}
	dir := filepath.Dir(abs)
	return loadedList{Entries: entries, BaseDir: dir, ListDir: dir, Label: displayPath(cwd, abs)}, nil
}

// loadRemoteList reads the list file at a GitHub URL.
func loadRemoteList(rawURL, cwd string, client *fetch.Client) (loadedList, error) {
	src, err := resolveDownload(client, rawURL, "")
	if err != nil {
		return loadedList{}, fmt.Errorf("the list URL: %w", err)
	}
	if src.Path == "" {
		return loadedList{}, fmt.Errorf("%w: %s", ErrListNotFile, rawURL)
	}
	data, err := client.File(src)
	if err != nil {
		return loadedList{}, fmt.Errorf("loading the list file: %w", err)
	}
	entries, err := manifest.Parse(bytes.NewReader(data))
	if err != nil {
		return loadedList{}, fmt.Errorf("loading the list file: %s: %w", src.URL, err)
	}
	return loadedList{Entries: entries, BaseDir: cwd, Label: src.URL}, nil
}
