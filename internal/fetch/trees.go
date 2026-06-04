package fetch

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

type treeResponse struct {
	Tree      []treeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

type treeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
}

// listFolder returns the blob entries that live inside src.Path (recursively),
// using the Trees API at the resolved commit. ok is false when the listing was
// truncated (huge repo), signalling the caller to use the tarball path.
func (c *Client) listFolder(src ghurl.Source) (entries []treeEntry, ok bool, err error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		apiBase, src.Owner, src.Repo, url.PathEscape(src.Commit))

	body, err := c.get(u, "application/vnd.github+json")
	if err != nil {
		return nil, false, err
	}
	var tr treeResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, false, err
	}
	if tr.Truncated {
		return nil, false, nil
	}

	folder := strings.Trim(src.Path, "/")
	for _, e := range tr.Tree {
		if e.Type != "blob" {
			continue
		}
		// Empty folder => whole repo: keep every blob.
		if folder == "" || e.Path == folder || strings.HasPrefix(e.Path, folder+"/") {
			entries = append(entries, e)
		}
	}
	return entries, true, nil
}

// downloadViaTrees fetches each blob under the folder from raw.githubusercontent
// into destDir. A rate-limit error is returned unwrapped so the orchestrator can
// fall back to the tarball.
func (c *Client) downloadViaTrees(src ghurl.Source, entries []treeEntry, destDir string, warn func(string)) error {
	base := strings.Trim(src.Path, "/")
	for _, e := range entries {
		rel := strings.TrimPrefix(e.Path, base)
		rel = strings.TrimPrefix(rel, "/")

		rawURL := fmt.Sprintf("%s/%s/%s/%s/%s",
			rawBase, src.Owner, src.Repo, url.PathEscape(src.Commit), escapePath(e.Path))

		data, err := c.get(rawURL, "")
		if err != nil {
			return err // includes rateLimitError for fallback
		}

		dest, err := safeJoin(destDir, rel)
		if err != nil {
			return err
		}

		if e.Mode == "120000" { // symlink: blob content is the target path
			if err := writeSymlink(dest, strings.TrimSpace(string(data)), warn); err != nil {
				return err
			}
			continue
		}
		if err := writeFile(dest, data, e.Mode == "100755"); err != nil {
			return err
		}
	}
	return nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
