package fetch

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

var (
	errTarballRequest = errors.New("tarball request failed")
	errFolderNotFound = errors.New("folder not found in repository")
)

// downloadViaTarball streams the repo tarball at the resolved commit and writes
// only the entries inside src.Path into destDir. This is a single request, used
// for large folders or when per-file fetching is rate-limited.
//
// Anonymous clients (and any client past the API rate limit) fetch it straight
// from codeload, which does not count against the API quota. With a token the
// API endpoint is used while it allows, since only it serves private repos.
func (c *Client) downloadViaTarball(src ghurl.Source, destDir string, warn func(string)) error {
	var resp *http.Response
	var err error
	if c.token != "" && !c.isLimited() {
		resp, err = c.do(fmt.Sprintf("%s/repos/%s/%s/tarball/%s", apiBase, src.Owner, src.Repo, src.Commit), "")
	} else {
		u := fmt.Sprintf("%s/%s/%s/tar.gz/%s", codeloadBase, src.Owner, src.Repo, url.PathEscape(src.Commit))
		resp, err = c.send(u, func(http.Header) {})
	}
	if err != nil {
		return err
	}
	defer c.closeBody(resp.Body)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return rateLimitError{status: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTP %d", errTarballRequest, resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("opening tarball: %w", err)
	}
	defer c.closeBody(gz)

	// GitHub tarballs wrap everything in a top-level "owner-repo-sha/" dir.
	// We strip that and keep only entries under our folder path (empty folder =>
	// whole repo).
	folder := strings.Trim(src.Path, "/")
	tr := tar.NewReader(gz)
	wrote := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tarball: %w", err)
		}

		rel := stripTopLevel(hdr.Name)
		if rel == "" {
			continue
		}

		inner, ok := folderMember(folder, rel)
		if !ok || inner == "" {
			continue // outside the folder, or the folder entry itself
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
				return fmt.Errorf("reading tarball entry %s: %w", hdr.Name, err)
			}
			exec := hdr.Mode&0o111 != 0
			if err := writeFile(dest, data, exec); err != nil {
				return err
			}
			wrote++
		}
	}

	if wrote == 0 {
		return fmt.Errorf("%w: %s", errFolderNotFound, src.Path)
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
