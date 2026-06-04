package fetch

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

// downloadViaTarball streams the repo tarball at the resolved commit and writes
// only the entries inside src.Path into destDir. This is a single request, used
// for large folders or when per-file fetching is rate-limited.
func (c *Client) downloadViaTarball(src ghurl.Source, destDir string, warn func(string)) error {
	u := fmt.Sprintf("%s/repos/%s/%s/tarball/%s", apiBase, src.Owner, src.Repo, src.Commit)

	resp, err := c.do(u, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return rateLimitError{status: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tarball request failed: HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()

	// GitHub tarballs wrap everything in a top-level "owner-repo-sha/" dir.
	// We strip that and keep only entries under our folder path (empty folder =>
	// whole repo).
	folder := strings.Trim(src.Path, "/")
	tr := tar.NewReader(gz)
	wrote := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		rel := stripTopLevel(hdr.Name)
		if rel == "" {
			continue
		}

		var inner string
		if folder == "" {
			inner = rel
		} else {
			if rel != folder && !strings.HasPrefix(rel, folder+"/") {
				continue
			}
			inner = strings.TrimPrefix(strings.TrimPrefix(rel, folder), "/")
		}
		if inner == "" {
			continue // the folder entry itself
		}

		dest, err := safeJoin(destDir, inner)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue // directories are created on demand by writeFile
		case tar.TypeSymlink, tar.TypeLink:
			if err := writeSymlink(dest, hdr.Linkname, warn); err != nil {
				return err
			}
			wrote++
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			exec := hdr.Mode&0o111 != 0
			if err := writeFile(dest, data, exec); err != nil {
				return err
			}
			wrote++
		}
	}

	if wrote == 0 {
		return fmt.Errorf("folder not found in repository: %s", src.Path)
	}
	return nil
}

// stripTopLevel removes the leading "owner-repo-sha/" component GitHub adds.
func stripTopLevel(name string) string {
	name = strings.TrimPrefix(name, "./")
	i := strings.IndexByte(name, '/')
	if i < 0 {
		return ""
	}
	return name[i+1:]
}
