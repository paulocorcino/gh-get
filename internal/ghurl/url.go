// Package ghurl parses GitHub "folder" URLs and resolves the branch/path
// ambiguity that arises when a branch name itself contains slashes.
package ghurl

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	errNotGitHub     = errors.New("not a github.com URL")
	errMissingRef    = errors.New("missing ref")
	errBadShape      = errors.New("unexpected URL shape, want https://github.com/OWNER/REPO[/tree/REF[/FOLDER]]")
	errNoDefault     = errors.New("not found")
	errRefUnresolved = errors.New("could not be resolved to a commit")
	errUnresolved    = errors.New("could not resolve a branch/tag")
)

// Partial is the result of parsing a GitHub URL before the ref (branch/tag)
// has been separated from the folder path. Splitting the two requires knowing
// which prefixes are real refs, which is resolved via Resolve.
type Partial struct {
	Owner string
	Repo  string
	rest  []string // path segments after tree|blob
}

// Source is a fully resolved reference to a folder inside a GitHub repo.
type Source struct {
	Owner  string
	Repo   string
	Ref    string // branch or tag (may contain "/")
	Path   string // folder path within the repo (no leading/trailing slash)
	Commit string // resolved commit SHA at fetch time
	URL    string // canonical https://github.com/owner/repo/tree/ref/path
}

// RefChecker reports whether ref exists in the repo and, if so, the commit SHA
// it points to. It is injected so URL resolution stays free of network code.
type RefChecker func(ref string) (sha string, ok bool, err error)

// RefLister returns every branch and tag named prefix or nested under
// prefix+"/", mapped to its commit SHA ("" when unknown, e.g. annotated tags).
// complete is false when the listing may be partial. It lets Resolve split a
// deep URL with a fixed number of requests instead of one probe per segment.
type RefLister func(prefix string) (refs map[string]string, complete bool, err error)

// Parse extracts owner, repo and the remaining path segments from a GitHub URL.
// Accepted shapes (query strings and trailing slashes are ignored):
//
//	https://github.com/OWNER/REPO                       (whole repo, default branch)
//	https://github.com/OWNER/REPO/tree/REF              (whole repo at REF)
//	https://github.com/OWNER/REPO/tree/REF/FOLDER/PATH  (a folder)
//	https://github.com/OWNER/REPO/blob/REF/FOLDER/PATH
func Parse(raw string) (Partial, error) {
	raw = trimURL(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return Partial{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host != "github.com" {
		return Partial{}, fmt.Errorf("%w: %s", errNotGitHub, raw)
	}

	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return Partial{}, badShape(raw)
	}

	rest, err := refSegments(segs[2:], raw)
	if err != nil {
		return Partial{}, err
	}
	return Partial{Owner: segs[0], Repo: segs[1], rest: rest}, nil
}

// trimURL drops surrounding space, the query string, the fragment and trailing
// slashes from raw.
func trimURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimRight(raw, "/")
}

// refSegments checks the URL path segments after OWNER/REPO: none (a bare repo
// URL), or tree|blob followed by at least the ref. It returns the segments
// after tree|blob.
func refSegments(segs []string, raw string) ([]string, error) {
	if len(segs) == 0 {
		return nil, nil
	}
	if segs[0] != "tree" && segs[0] != "blob" {
		return nil, badShape(raw)
	}
	if len(segs) == 1 {
		return nil, fmt.Errorf("%w after /%s/: %s", errMissingRef, segs[0], raw)
	}
	return segs[1:], nil
}

func badShape(raw string) error {
	return fmt.Errorf("%w: %s", errBadShape, raw)
}

// Resolve separates the ref from the (possibly empty) folder path. When the URL
// carries no ref at all (bare repo URL), defaultBranch supplies it. Because a
// branch may contain slashes, candidate refs are tried longest-first so
// "feature/x" wins over "feature"; an empty path (whole repo) is allowed. The
// matching ref's commit SHA is captured into the returned Source.
//
// When the URL has more than one segment after tree/blob and list is non-nil,
// the candidates come from a single listing of refs sharing the first segment;
// check is then only needed for a commit SHA in the URL or an annotated tag.
// Without list (or with an incomplete listing) each prefix is probed via check.
func (p Partial) Resolve(check RefChecker, list RefLister, defaultBranch func() (string, error)) (Source, error) {
	if len(p.rest) == 0 {
		return p.resolveDefault(check, defaultBranch)
	}

	if list != nil && len(p.rest) > 1 {
		refs, complete, err := list(p.rest[0])
		if err != nil {
			return Source{}, err
		}
		if complete {
			return p.resolveListed(refs, check)
		}
	}

	return p.resolveProbed(check)
}

// resolveDefault resolves a bare repo URL to the whole repo at the default
// branch.
func (p Partial) resolveDefault(check RefChecker, defaultBranch func() (string, error)) (Source, error) {
	ref, err := defaultBranch()
	if err != nil {
		return Source{}, err
	}
	sha, ok, err := check(ref)
	if err != nil {
		return Source{}, err
	}
	if !ok {
		return Source{}, fmt.Errorf("default branch %q %w", ref, errNoDefault)
	}
	return Build(p.Owner, p.Repo, ref, "", sha), nil
}

// resolveProbed probes each ref prefix of the URL via check, longest first;
// the path may be empty (whole repo at that ref).
func (p Partial) resolveProbed(check RefChecker) (Source, error) {
	for i := len(p.rest); i >= 1; i-- {
		ref := strings.Join(p.rest[:i], "/")
		path := strings.Join(p.rest[i:], "/")

		sha, ok, err := check(ref)
		if err != nil {
			return Source{}, err
		}
		if ok {
			return Build(p.Owner, p.Repo, ref, path, sha), nil
		}
	}

	return Source{}, p.unresolved()
}

// resolveListed picks the longest listed ref that prefixes the URL. When no
// branch or tag matches, the first segment can still be a commit SHA (which
// never contains "/"), so it alone is checked.
func (p Partial) resolveListed(refs map[string]string, check RefChecker) (Source, error) {
	for i := len(p.rest); i >= 1; i-- {
		ref := strings.Join(p.rest[:i], "/")
		sha, listed := refs[ref]
		if !listed {
			continue
		}
		sha, err := peeled(check, ref, sha)
		if err != nil {
			return Source{}, err
		}
		return Build(p.Owner, p.Repo, ref, strings.Join(p.rest[i:], "/"), sha), nil
	}

	sha, ok, err := check(p.rest[0])
	if err != nil {
		return Source{}, err
	}
	if !ok {
		return Source{}, p.unresolved()
	}
	return Build(p.Owner, p.Repo, p.rest[0], strings.Join(p.rest[1:], "/"), sha), nil
}

// peeled returns the commit of a listed ref: sha itself, or for an annotated
// tag (listed with an empty sha) the commit check resolves it to.
func peeled(check RefChecker, ref, sha string) (string, error) {
	if sha != "" {
		return sha, nil
	}
	resolved, ok, err := check(ref)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("ref %q %w", ref, errRefUnresolved)
	}
	return resolved, nil
}

func (p Partial) unresolved() error {
	return fmt.Errorf("%w in %s/%s from URL", errUnresolved, p.Owner, p.Repo)
}

// Build assembles a Source directly (used when the ref/commit are already known,
// e.g. an explicit --ref override or an update from a stored marker).
func Build(owner, repo, ref, path, commit string) Source {
	return Source{
		Owner:  owner,
		Repo:   repo,
		Ref:    ref,
		Path:   path,
		Commit: commit,
		URL:    canonical(owner, repo, ref, path),
	}
}

func canonical(owner, repo, ref, path string) string {
	if path == "" {
		return fmt.Sprintf("https://github.com/%s/%s/tree/%s", owner, repo, ref)
	}
	return fmt.Sprintf("https://github.com/%s/%s/tree/%s/%s", owner, repo, ref, path)
}
