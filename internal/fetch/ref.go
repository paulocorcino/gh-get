package fetch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// CheckRef reports whether ref (branch, tag or SHA) exists in owner/repo and,
// if so, the commit SHA it resolves to. It uses the lightweight ".sha" media
// type so only the SHA is transferred. Designed to satisfy ghurl.RefChecker.
// Answers are cached for the life of the Client.
func (c *Client) CheckRef(owner, repo string) func(ref string) (string, bool, error) {
	return func(ref string) (string, bool, error) {
		key := cacheKey(owner, repo, ref)
		c.mu.Lock()
		r, hit := c.refs[key]
		c.mu.Unlock()
		if hit {
			return r.sha, r.ok, nil
		}

		sha, ok, err := c.lookupRef(owner, repo, ref)
		if err != nil {
			return "", false, err
		}
		c.mu.Lock()
		c.refs[key] = refResult{sha: sha, ok: ok}
		c.mu.Unlock()
		return sha, ok, nil
	}
}

// lookupRef asks the API and, once it is rate-limited, the git ref
// advertisement instead.
func (c *Client) lookupRef(owner, repo, ref string) (string, bool, error) {
	if !c.isLimited() {
		sha, ok, err := c.checkRef(owner, repo, ref)
		if !c.noteLimit(err) {
			return sha, ok, err
		}
	}
	adv, err := c.advertised(owner, repo)
	if err != nil {
		return "", false, err
	}
	sha, ok := adv.resolve(ref)
	return sha, ok, nil
}

func (c *Client) checkRef(owner, repo, ref string) (string, bool, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", apiBase, owner, repo, refEscape(ref))
	resp, err := c.do(u, "application/vnd.github.sha")
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		buf, err := io.ReadAll(io.LimitReader(resp.Body, 128))
		if err != nil {
			return "", false, err
		}
		return strings.TrimSpace(string(buf)), true, nil
	case http.StatusNotFound, http.StatusUnprocessableEntity:
		return "", false, nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		return "", false, rateLimitError{status: resp.StatusCode}
	default:
		return "", false, fmt.Errorf("unexpected status HTTP %d resolving ref %q", resp.StatusCode, ref)
	}
}

// ListRefs returns the branches and tags named prefix or nested under
// prefix+"/", mapped to the commit each points to ("" when only a tag object is
// known, i.e. annotated tags). complete is false when GitHub signals more
// pages. Two requests replace one probe per URL segment. Designed to satisfy
// ghurl.RefLister.
func (c *Client) ListRefs(owner, repo string) func(prefix string) (map[string]string, bool, error) {
	return func(prefix string) (map[string]string, bool, error) {
		key := cacheKey(owner, repo, prefix)
		c.mu.Lock()
		refs, hit := c.lists[key]
		c.mu.Unlock()
		if hit {
			return refs, true, nil
		}

		refs, complete, err := c.matchingRefs(owner, repo, prefix)
		if err != nil || !complete {
			return nil, complete, err
		}
		c.mu.Lock()
		c.lists[key] = refs
		c.mu.Unlock()
		return refs, true, nil
	}
}

// matchingRefs lists refs under prefix via the API and, once it is
// rate-limited, via the git ref advertisement instead.
func (c *Client) matchingRefs(owner, repo, prefix string) (map[string]string, bool, error) {
	if !c.isLimited() {
		refs, complete, err := c.apiMatchingRefs(owner, repo, prefix)
		if !c.noteLimit(err) {
			return refs, complete, err
		}
	}
	adv, err := c.advertised(owner, repo)
	if err != nil {
		return nil, false, err
	}
	return adv.matching(prefix), true, nil
}

func (c *Client) apiMatchingRefs(owner, repo, prefix string) (map[string]string, bool, error) {
	refs := map[string]string{}
	// Tags last so a tag shadows a same-named branch, as git's rev-parse
	// order (and so CheckRef) does.
	for _, kind := range []string{"heads", "tags"} {
		u := fmt.Sprintf("%s/repos/%s/%s/git/matching-refs/%s/%s",
			apiBase, owner, repo, kind, refEscape(prefix))
		body, hdr, err := c.getWithHeader(u, "application/vnd.github+json")
		if err != nil {
			return nil, false, err
		}
		if strings.Contains(hdr.Get("Link"), `rel="next"`) {
			return nil, false, nil
		}
		if err := collectRefs(body, kind, prefix, refs); err != nil {
			return nil, false, err
		}
	}
	return refs, true, nil
}

// collectRefs decodes a matching-refs response of the given kind ("heads" or
// "tags") into refs, keeping only names equal to prefix or nested under it
// (matching-refs is a raw string prefix, so "main" also returns "maintenance").
func collectRefs(body []byte, kind, prefix string, refs map[string]string) error {
	var list []struct {
		Ref    string `json:"ref"`
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return err
	}
	for _, r := range list {
		name := strings.TrimPrefix(r.Ref, "refs/"+kind+"/")
		if name != prefix && !strings.HasPrefix(name, prefix+"/") {
			continue
		}
		sha := r.Object.SHA
		if r.Object.Type != "commit" {
			sha = "" // annotated tag: the tag object, not the commit
		}
		refs[name] = sha
	}
	return nil
}

func cacheKey(owner, repo, ref string) string {
	return strings.ToLower(owner) + "/" + strings.ToLower(repo) + "\x00" + ref
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
