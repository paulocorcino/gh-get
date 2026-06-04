package fetch

import (
	"encoding/json"
	"fmt"
)

// DefaultBranch returns the repository's default branch (e.g. "main"), used when
// a bare repo URL carries no ref.
func (c *Client) DefaultBranch(owner, repo string) (string, error) {
	u := fmt.Sprintf("%s/repos/%s/%s", apiBase, owner, repo)
	body, err := c.get(u, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var r struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.DefaultBranch == "" {
		return "", fmt.Errorf("could not determine default branch for %s/%s", owner, repo)
	}
	return r.DefaultBranch, nil
}
