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
	resp, err := c.requestTarball(src)
	if err != nil {
		return err
	}
	defer c.closeBody(resp.Body)

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("opening tarball: %w", err)
	}
	defer c.closeBody(gz)

	// GitHub tarballs wrap everything in a top-level "owner-repo-sha/" dir.
	// We strip that and keep only entries under our folder path (empty folder =>
	// whole repo).
	wrote, err := extractFolder(tar.NewReader(gz), strings.Trim(src.Path, "/"), destDir, warn)
	if err != nil {
		return err
	}
	if wrote == 0 {
		return fmt.Errorf("%w: %s", errFolderNotFound, src.Path)
	}
	return nil
}

// requestTarball sends the tarball request and returns the response once its
// status is a success; on any other status it closes the body itself.
func (c *Client) requestTarball(src ghurl.Source) (*http.Response, error) {
	var resp *http.Response
	var err error
	if c.token != "" && !c.isLimited() {
		resp, err = c.do(fmt.Sprintf("%s/repos/%s/%s/tarball/%s", apiBase, src.Owner, src.Repo, src.Commit), "")
	} else {
		u := fmt.Sprintf("%s/%s/%s/tar.gz/%s", codeloadBase, src.Owner, src.Repo, url.PathEscape(src.Commit))
		resp, err = c.send(u, func(http.Header) {})
	}
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		c.closeBody(resp.Body)
		return nil, rateLimitError{status: resp.StatusCode}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		c.closeBody(resp.Body)
		return nil, fmt.Errorf("%w: HTTP %d", errTarballRequest, resp.StatusCode)
	}
	return resp, nil
}

// extractFolder writes every tarball entry inside folder into destDir and
// returns how many files and links it wrote.
func extractFolder(tr *tar.Reader, folder, destDir string, warn func(string)) (int, error) {
	wrote := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return wrote, nil
		}
		if err != nil {
			return wrote, fmt.Errorf("reading tarball: %w", err)
		}
		n, err := extractEntry(tr, hdr, folder, destDir, warn)
		if err != nil {
			return wrote, err
		}
		wrote += n
	}
}

// extractEntry writes the current tarball entry under destDir when it lies
// inside folder, and returns how many files or links it wrote (0 or 1).
func extractEntry(r io.Reader, hdr *tar.Header, folder, destDir string, warn func(string)) (int, error) {
	dest, ok, err := entryDest(hdr.Name, folder, destDir)
	if err != nil || !ok {
		return 0, err
	}
	return writeTarEntry(r, hdr, dest, warn)
}

// entryDest maps a tarball entry name to its path under destDir. ok is false
// for entries outside folder and for the folder entry itself.
func entryDest(name, folder, destDir string) (string, bool, error) {
	rel := stripTopLevel(name)
	if rel == "" {
		return "", false, nil
	}
	inner, ok := folderMember(folder, rel)
	if !ok || inner == "" {
		return "", false, nil // outside the folder, or the folder entry itself
	}
	dest, err := safeJoin(destDir, inner)
	if err != nil {
		return "", false, err
	}
	return dest, true, nil
}

// writeTarEntry writes one tarball entry at dest. It returns 1 when it wrote a
// file or link and 0 for an entry it skips.
func writeTarEntry(r io.Reader, hdr *tar.Header, dest string, warn func(string)) (int, error) {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return 0, nil // directories are created on demand by writeFile
	case tar.TypeSymlink, tar.TypeLink:
		if err := writeSymlink(dest, hdr.Linkname, warn); err != nil {
			return 0, err
		}
		return 1, nil
	case tar.TypeReg:
		data, err := io.ReadAll(r)
		if err != nil {
			return 0, fmt.Errorf("reading tarball entry %s: %w", hdr.Name, err)
		}
		if err := writeFile(dest, data, hdr.Mode&0o111 != 0); err != nil {
			return 0, err
		}
		return 1, nil
	}
	return 0, nil
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
