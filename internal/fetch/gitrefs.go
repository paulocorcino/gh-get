package fetch

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var (
	errNotFoundBare = errors.New("not found")
	errTruncated    = errors.New("truncated pkt-line")
	errBadPktLength = errors.New("bad pkt-line length")
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
	defer c.closeBody(resp.Body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound:
		return advertisement{}, fmt.Errorf("repository %s/%s %w (private repos need a token)", owner, repo, errNotFoundBare)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return advertisement{}, fmt.Errorf("%w HTTP %d listing refs of %s/%s", errUnexpectedStatus, resp.StatusCode, owner, repo)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return advertisement{}, fmt.Errorf("reading refs of %s/%s: %w", owner, repo, err)
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
		var line []byte
		var err error
		if line, data, err = nextPktLine(data); err != nil {
			return advertisement{}, err
		}
		addAdvertised(&adv, line)
	}
	return adv, nil
}

// nextPktLine splits the first pkt-line off data and returns its payload
// without the trailing newline, and the data after it. A flush-pkt has an
// empty payload.
func nextPktLine(data []byte) ([]byte, []byte, error) {
	if len(data) < 4 {
		return nil, nil, errTruncated
	}
	n, err := strconv.ParseUint(string(data[:4]), 16, 16)
	if err != nil {
		return nil, nil, fmt.Errorf("%w %q", errBadPktLength, data[:4])
	}
	if n == 0 { // flush-pkt
		return nil, data[4:], nil
	}
	if n < 4 || int(n) > len(data) {
		return nil, nil, fmt.Errorf("%w %d", errBadPktLength, n)
	}
	return bytes.TrimSuffix(data[4:n], []byte("\n")), data[n:], nil
}

// addAdvertised records one advertisement line in adv: the HEAD symref from
// the capabilities after a NUL, and the "<sha> <ref>" pair. Service headers,
// placeholders and malformed lines add nothing.
func addAdvertised(adv *advertisement, line []byte) {
	if bytes.HasPrefix(line, []byte("# ")) {
		return
	}
	if i := bytes.IndexByte(line, 0); i >= 0 {
		if head, ok := headSymref(string(line[i+1:])); ok {
			adv.head = head
		}
		line = line[:i]
	}
	sha, name, ok := strings.Cut(string(line), " ")
	if !ok || !isHexSHA(sha) || name == "capabilities^{}" {
		return // empty repo placeholder or malformed line
	}
	if base, peeled := strings.CutSuffix(name, "^{}"); peeled {
		adv.refs[base] = sha // annotated tag: keep the commit it points to
		return
	}
	adv.refs[name] = sha
}

// headSymref returns the default branch named by the last symref=HEAD
// capability, if any.
func headSymref(capabilities string) (string, bool) {
	head, found := "", false
	for capa := range strings.FieldsSeq(capabilities) {
		if target, ok := strings.CutPrefix(capa, "symref=HEAD:"); ok {
			head, found = strings.TrimPrefix(target, "refs/heads/"), true
		}
	}
	return head, found
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
