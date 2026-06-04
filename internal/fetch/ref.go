package fetch

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CheckRef reports whether ref (branch, tag or SHA) exists in owner/repo and,
// if so, the commit SHA it resolves to. It uses the lightweight ".sha" media
// type so only the SHA is transferred. Designed to satisfy ghurl.RefChecker.
func (c *Client) CheckRef(owner, repo string) func(ref string) (string, bool, error) {
	return func(ref string) (string, bool, error) {
		u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", apiBase, owner, repo, refEscape(ref))
		resp, err := c.do(u, "application/vnd.github.sha")
		if err != nil {
			return "", false, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			buf := make([]byte, 64)
			n, _ := resp.Body.Read(buf)
			return strings.TrimSpace(string(buf[:n])), true, nil
		case http.StatusNotFound, http.StatusUnprocessableEntity:
			return "", false, nil
		case http.StatusForbidden, http.StatusTooManyRequests:
			return "", false, rateLimitError{status: resp.StatusCode}
		default:
			return "", false, fmt.Errorf("unexpected status HTTP %d resolving ref %q", resp.StatusCode, ref)
		}
	}
}

// refEscape percent-encodes each path segment of a ref so slashes are preserved
// as path separators while other special characters are escaped.
func refEscape(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
