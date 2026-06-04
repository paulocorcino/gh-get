// Package ghurl parses GitHub "folder" URLs and resolves the branch/path
// ambiguity that arises when a branch name itself contains slashes.
package ghurl

import (
	"fmt"
	"net/url"
	"strings"
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

// Parse extracts owner, repo and the remaining path segments from a GitHub URL.
// Accepted shapes (query strings and trailing slashes are ignored):
//
//	https://github.com/OWNER/REPO                       (whole repo, default branch)
//	https://github.com/OWNER/REPO/tree/REF              (whole repo at REF)
//	https://github.com/OWNER/REPO/tree/REF/FOLDER/PATH  (a folder)
//	https://github.com/OWNER/REPO/blob/REF/FOLDER/PATH
func Parse(raw string) (Partial, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimRight(raw, "/")

	u, err := url.Parse(raw)
	if err != nil {
		return Partial{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host != "github.com" {
		return Partial{}, fmt.Errorf("not a github.com URL: %s", raw)
	}

	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return Partial{}, badShape(raw)
	}

	p := Partial{Owner: segs[0], Repo: segs[1]}
	if len(segs) > 2 {
		if segs[2] != "tree" && segs[2] != "blob" {
			return Partial{}, badShape(raw)
		}
		p.rest = segs[3:]
		if len(p.rest) == 0 {
			return Partial{}, fmt.Errorf("missing ref after /%s/: %s", segs[2], raw)
		}
	}
	return p, nil
}

func badShape(raw string) error {
	return fmt.Errorf(
		"unexpected URL shape, want https://github.com/OWNER/REPO[/tree/REF[/FOLDER]]: %s", raw)
}

// Resolve separates the ref from the (possibly empty) folder path. When the URL
// carries no ref at all (bare repo URL), defaultBranch supplies it. Because a
// branch may contain slashes, candidate refs are tried longest-first so
// "feature/x" wins over "feature"; an empty path (whole repo) is allowed. The
// matching ref's commit SHA is captured into the returned Source.
func (p Partial) Resolve(check RefChecker, defaultBranch func() (string, error)) (Source, error) {
	if len(p.rest) == 0 {
		ref, err := defaultBranch()
		if err != nil {
			return Source{}, err
		}
		sha, ok, err := check(ref)
		if err != nil {
			return Source{}, err
		}
		if !ok {
			return Source{}, fmt.Errorf("default branch %q not found", ref)
		}
		return Build(p.Owner, p.Repo, ref, "", sha), nil
	}

	// Longest ref prefix first; path may be empty (whole repo at that ref).
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

	return Source{}, fmt.Errorf("could not resolve a branch/tag in %s/%s from URL", p.Owner, p.Repo)
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
