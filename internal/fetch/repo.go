package fetch

import (
	"encoding/json"
	"fmt"
)

// DefaultBranch returns the repository's default branch (e.g. "main"), used when
// a bare repo URL carries no ref.
// Once the API is rate-limited it is read from the git ref advertisement.
func (c *Client) DefaultBranch(owner, repo string) (string, error) {
	if !c.isLimited() {
		branch, err := c.apiDefaultBranch(owner, repo)
		if !c.noteLimit(err) {
			return branch, err
		}
	}
	adv, err := c.advertised(owner, repo)
	if err != nil {
		return "", err
	}
	if adv.head == "" {
		return "", fmt.Errorf("could not determine default branch for %s/%s", owner, repo)
	}
	return adv.head, nil
}

func (c *Client) apiDefaultBranch(owner, repo string) (string, error) {
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
