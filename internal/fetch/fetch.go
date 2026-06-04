package fetch

import (
	"github.com/paulocorcino/gh-get/internal/ghurl"
)

// Download writes the folder referenced by src into destDir, which must already
// exist. It uses the minimal Trees+raw path by default and falls back to the
// repo tarball when the folder is large, the tree listing is truncated, or the
// API rate-limits per-file fetches. warn (optional) receives non-fatal notices.
func (c *Client) Download(src ghurl.Source, destDir string, warn func(string)) error {
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
