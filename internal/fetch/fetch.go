package fetch

import (
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
	// before writing. Used for fresh/--force downloads and for update.
	Replace
)

// Materialize downloads the folder referenced by src into a temporary area and
// only then publishes it into destDir according to mode. Because the destination
// is touched only after a fully successful download, any failure leaves destDir
// untouched. warn (optional) receives non-fatal notices.
func (c *Client) Materialize(src ghurl.Source, destDir string, mode WriteMode, warn func(string)) error {
	tmp, err := os.MkdirTemp("", "gh-get-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := c.download(src, tmp, warn); err != nil {
		return err
	}

	if mode == Replace {
		if err := clearContents(destDir); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return publishTree(tmp, destDir, warn)
}

// download writes the folder referenced by src into destDir, which must already
// exist. It uses the minimal Trees+raw path by default and falls back to the
// repo tarball when the folder is large, the tree listing is truncated, or the
// API rate-limits per-file fetches. warn (optional) receives non-fatal notices.
func (c *Client) download(src ghurl.Source, destDir string, warn func(string)) error {
	entries, ok, err := c.listFolder(src)
	switch {
	case isRateLimit(err):
		return c.downloadViaTarball(src, destDir, warn)
	case err != nil:
		return err
	case !ok:
		// Tree listing was truncated (very large repo).
		return c.downloadViaTarball(src, destDir, warn)
	}

	if len(entries) == 0 {
		return errFolderNotFound{path: src.Path}
	}

	if len(entries) > tarballThreshold {
		return c.downloadViaTarball(src, destDir, warn)
	}

	if err := c.downloadViaTrees(src, entries, destDir, warn); err != nil {
		if isRateLimit(err) {
			if warn != nil {
				warn("rate limited during per-file download, retrying via tarball")
			}
			return c.downloadViaTarball(src, destDir, warn)
		}
		return err
	}
	return nil
}

type errFolderNotFound struct{ path string }

func (e errFolderNotFound) Error() string {
	return "folder not found in repository: " + e.path
}
