package fetch

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// advertisement is a repo's ref advertisement from the git smart-HTTP endpoint
// (what "git ls-remote" reads). It needs no git binary and does not count
// against the REST API rate limit, so it backs ref resolution once the API
// refuses requests.
type advertisement struct {
	refs map[string]string // full ref name -> commit SHA (annotated tags peeled)
	head string            // default branch, from the HEAD symref
}

// advertised returns the (cached) ref advertisement of owner/repo.
func (c *Client) advertised(owner, repo string) (advertisement, error) {
	key := cacheKey(owner, repo, "")
	c.mu.Lock()
	adv, hit := c.gitRefs[key]
	c.mu.Unlock()
	if hit {
		return adv, nil
	}

	u := fmt.Sprintf("%s/%s/%s.git/info/refs?service=git-upload-pack", gitBase, owner, repo)
	resp, err := c.send(u, func(h http.Header) {
		if c.token != "" {
			h.Set("Authorization", "Basic "+basicToken(c.token))
		}
	})
	if err != nil {
		return advertisement{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound:
		return advertisement{}, fmt.Errorf("repository %s/%s not found (private repos need a token)", owner, repo)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return advertisement{}, fmt.Errorf("unexpected status HTTP %d listing refs of %s/%s", resp.StatusCode, owner, repo)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return advertisement{}, err
	}
	if adv, err = parseAdvertisement(body); err != nil {
		return advertisement{}, fmt.Errorf("listing refs of %s/%s: %w", owner, repo, err)
	}
	c.mu.Lock()
	c.gitRefs[key] = adv
	c.mu.Unlock()
	return adv, nil
}

// resolve maps a branch, tag or commit SHA to a commit, in git's rev-parse
// order (tags shadow same-named branches). A hex string that is no known ref is
// taken as a commit SHA, since the advertisement cannot vouch for commits; a
// wrong one fails later at download.
func (a advertisement) resolve(ref string) (string, bool) {
	if sha, ok := a.refs["refs/tags/"+ref]; ok {
		return sha, true
	}
	if sha, ok := a.refs["refs/heads/"+ref]; ok {
		return sha, true
	}
	if isHexSHA(ref) {
		return ref, true
	}
	return "", false
}

// matching returns branches and tags named prefix or nested under prefix+"/",
// mapped to their commit, in the shape of ListRefs.
func (a advertisement) matching(prefix string) map[string]string {
	out := map[string]string{}
	for _, kind := range []string{"refs/heads/", "refs/tags/"} { // tags last: they shadow
		for full, sha := range a.refs {
			name, ok := strings.CutPrefix(full, kind)
			if ok && (name == prefix || strings.HasPrefix(name, prefix+"/")) {
				out[name] = sha
			}
		}
	}
	return out
}

// parseAdvertisement decodes the pkt-line ref advertisement of git's smart
// HTTP protocol (v0): an optional "# service=..." section, then one
// "<sha> <ref>" line per ref, the first carrying capabilities after a NUL.
func parseAdvertisement(data []byte) (advertisement, error) {
	adv := advertisement{refs: map[string]string{}}
	for len(data) > 0 {
		if len(data) < 4 {
			return advertisement{}, fmt.Errorf("truncated pkt-line")
		}
		n, err := strconv.ParseUint(string(data[:4]), 16, 16)
		if err != nil {
			return advertisement{}, fmt.Errorf("bad pkt-line length %q", data[:4])
		}
		if n == 0 { // flush-pkt
			data = data[4:]
			continue
		}
		if n < 4 || int(n) > len(data) {
			return advertisement{}, fmt.Errorf("bad pkt-line length %d", n)
		}
		line := bytes.TrimSuffix(data[4:n], []byte("\n"))
		data = data[n:]
		if bytes.HasPrefix(line, []byte("# ")) {
			continue
		}

		if i := bytes.IndexByte(line, 0); i >= 0 {
			for _, capa := range strings.Fields(string(line[i+1:])) {
				if target, ok := strings.CutPrefix(capa, "symref=HEAD:"); ok {
					adv.head = strings.TrimPrefix(target, "refs/heads/")
				}
			}
			line = line[:i]
		}
		sha, name, ok := strings.Cut(string(line), " ")
		if !ok || !isHexSHA(sha) || name == "capabilities^{}" {
			continue // empty repo placeholder or malformed line
		}
		if base, peeled := strings.CutSuffix(name, "^{}"); peeled {
			adv.refs[base] = sha // annotated tag: keep the commit it points to
			continue
		}
		adv.refs[name] = sha
	}
	return adv, nil
}

// isHexSHA reports whether s looks like a (possibly abbreviated) commit SHA.
func isHexSHA(s string) bool {
	if len(s) < 7 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// basicToken encodes a token as git-over-HTTPS Basic credentials.
func basicToken(token string) string {
	return base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
}
