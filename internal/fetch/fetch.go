package fetch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

// WriteMode selects how Materialize publishes the downloaded tree into the
// destination directory.
type WriteMode int

const (
	// Merge overwrites colliding paths and leaves every other existing file in
	// place. Used for in-place downloads into the current directory.
	Merge WriteMode = iota
	// Replace clears the destination's contents (never the directory itself)
	// before writing. Used for fresh/--force downloads.
	Replace
	// Sync removes only the files a previous download wrote (the prev list
	// passed to Materialize) that the new tree no longer has, then merges.
	// Used for update, so unrelated content sharing the folder survives. A nil
	// prev (legacy marker without a file list) degrades to Merge.
	Sync
)

// Materialize downloads the folder referenced by src into a temporary area and
// only then publishes it into destDir according to mode. Because the destination
// is touched only after a fully successful download, any failure leaves destDir
// untouched. prev is only used by Sync. It returns the slash-separated paths of
// every file written, for recording in the marker. warn (optional) receives
// non-fatal notices.
func (c *Client) Materialize(src ghurl.Source, destDir string, mode WriteMode, prev []string, warn func(string)) ([]string, error) {
	tmp, err := os.MkdirTemp("", "gh-get-*")
	if err != nil {
		return nil, fmt.Errorf("creating temporary directory: %w", err)
	}
	defer func() {
		if rerr := os.RemoveAll(tmp); rerr != nil && warn != nil {
			warn(fmt.Sprintf("could not remove temporary directory %s: %v", tmp, rerr))
		}
	}()

	if dlErr := c.download(src, tmp, warn); dlErr != nil {
		return nil, dlErr
	}
	files, err := listFiles(tmp)
	if err != nil {
		return nil, err
	}

	if err := prepareDest(destDir, mode, prev, files); err != nil {
		return nil, err
	}
	if err := publishTree(tmp, destDir, warn); err != nil {
		return nil, err
	}
	return files, nil
}

// prepareDest readies destDir for publishing files according to mode: Replace
// clears it, Sync prunes what prev wrote and files no longer has, Merge leaves
// it as is. destDir exists afterwards.
func prepareDest(destDir string, mode WriteMode, prev, files []string) error {
	switch mode {
	case Replace:
		if err := clearContents(destDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	case Merge:
		// Merge overlays onto the existing files; nothing to clear first.
	case Sync:
		if err := pruneStale(destDir, prev, files); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating destination %s: %w", destDir, err)
	}
	return nil
}

// download writes the folder referenced by src into destDir, which must already
// exist. It uses the minimal Trees+raw path by default and falls back to the
// repo tarball when the folder is large, the tree listing is truncated, or the
// API rate-limits per-file fetches. warn (optional) receives non-fatal notices.
func (c *Client) download(src ghurl.Source, destDir string, warn func(string)) error {
	if c.isLimited() {
		// The tree listing would only be refused again; codeload needs no API.
		return c.downloadViaTarball(src, destDir, warn)
	}
	entries, ok, err := c.listFolder(src)
	switch {
	case c.noteLimit(err):
		return c.downloadViaTarball(src, destDir, warn)
	case err != nil:
		return err
	case !ok:
		// Tree listing was truncated (very large repo).
		return c.downloadViaTarball(src, destDir, warn)
	}

	if len(entries) == 0 {
		return folderNotFoundError{path: src.Path}
	}

	if len(entries) > tarballThreshold {
		return c.downloadViaTarball(src, destDir, warn)
	}

	if err := c.downloadViaTrees(src, entries, destDir, warn); err != nil {
		if isRateLimit(err) {
			if warn != nil {
				warn("rate limited during per-file download, retrying via tarball")
			}
			c.noteLimit(err)
			return c.downloadViaTarball(src, destDir, warn)
		}
		return err
	}
	return nil
}

type folderNotFoundError struct{ path string }

func (e folderNotFoundError) Error() string {
	return "folder not found in repository: " + e.path
}
