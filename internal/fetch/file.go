package fetch

import (
	"fmt"
	"net/url"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

// File returns the content of the file src.Path at src.Commit, read from
// raw.githubusercontent without writing anything to disk.
func (c *Client) File(src ghurl.Source) ([]byte, error) {
	data, err := c.rawContent(src, src.Path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", src.Path, err)
	}
	return data, nil
}

// rawContent fetches the file at path in src's repository, at src.Commit, from
// raw.githubusercontent.
func (c *Client) rawContent(src ghurl.Source, path string) ([]byte, error) {
	rawURL := fmt.Sprintf("%s/%s/%s/%s/%s",
		rawBase, src.Owner, src.Repo, url.PathEscape(src.Commit), escapePath(path))
	return c.get(rawURL, "")
}
