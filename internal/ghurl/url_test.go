package ghurl

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name           string
		in             string
		wantOwner      string
		wantRepo       string
		wantRestJoined string
		wantErr        bool
	}{
		{
			name:           "tree url",
			in:             "https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff",
			wantOwner:      "mattpocock",
			wantRepo:       "skills",
			wantRestJoined: "main/skills/productivity/handoff",
		},
		{
			name:           "blob url with query and trailing slash",
			in:             "https://github.com/o/r/blob/main/a/b/?tab=readme#L1",
			wantOwner:      "o",
			wantRepo:       "r",
			wantRestJoined: "main/a/b",
		},
		{
			name:           "bare repo root",
			in:             "https://github.com/o/r",
			wantOwner:      "o",
			wantRepo:       "r",
			wantRestJoined: "",
		},
		{
			name:           "tree with ref only (whole branch)",
			in:             "https://github.com/o/r/tree/main",
			wantOwner:      "o",
			wantRepo:       "r",
			wantRestJoined: "main",
		},
		{name: "non-github", in: "https://gitlab.com/o/r/tree/main/a", wantErr: true},
		{name: "tree without ref", in: "https://github.com/o/r/tree", wantErr: true},
		{name: "no tree/blob", in: "https://github.com/o/r/commits/main/a", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Parse(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Owner != tc.wantOwner || p.Repo != tc.wantRepo {
				t.Fatalf("owner/repo = %s/%s, want %s/%s", p.Owner, p.Repo, tc.wantOwner, tc.wantRepo)
			}
			if got := join(p.rest); got != tc.wantRestJoined {
				t.Fatalf("rest = %q, want %q", got, tc.wantRestJoined)
			}
		})
	}
}

// noDefault fails the test if the default-branch resolver is unexpectedly used.
func noDefault(t *testing.T) func() (string, error) {
	return func() (string, error) {
		t.Helper()
		t.Fatal("defaultBranch should not be called when URL carries a ref")
		return "", nil
	}
}

func TestResolve_SimpleBranch(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/main/skills/handoff")
	check := func(ref string) (string, bool, error) {
		if ref == "main" {
			return "abc123", true, nil
		}
		return "", false, nil
	}
	src, err := p.Resolve(check, nil, noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "main" || src.Path != "skills/handoff" || src.Commit != "abc123" {
		t.Fatalf("got ref=%q path=%q commit=%q", src.Ref, src.Path, src.Commit)
	}
	want := "https://github.com/o/r/tree/main/skills/handoff"
	if src.URL != want {
		t.Fatalf("url = %q, want %q", src.URL, want)
	}
}

func TestResolve_SlashedBranchPrefersLongest(t *testing.T) {
	// Both "feature" and "feature/x" exist; the longest valid ref must win.
	p, _ := Parse("https://github.com/o/r/tree/feature/x/skills/handoff")
	check := func(ref string) (string, bool, error) {
		switch ref {
		case "feature", "feature/x":
			return "sha-" + ref, true, nil
		}
		return "", false, nil
	}
	src, err := p.Resolve(check, nil, noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "feature/x" || src.Path != "skills/handoff" {
		t.Fatalf("got ref=%q path=%q", src.Ref, src.Path)
	}
}

func TestResolve_NoMatch(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/main/a")
	check := func(string) (string, bool, error) { return "", false, nil }
	if _, err := p.Resolve(check, nil, noDefault(t)); err == nil {
		t.Fatal("expected error when no ref matches")
	}
}

func TestResolve_WholeBranch(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/main")
	check := func(ref string) (string, bool, error) {
		if ref == "main" {
			return "sha", true, nil
		}
		return "", false, nil
	}
	src, err := p.Resolve(check, nil, noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "main" || src.Path != "" {
		t.Fatalf("got ref=%q path=%q", src.Ref, src.Path)
	}
	if src.URL != "https://github.com/o/r/tree/main" {
		t.Fatalf("url = %q", src.URL)
	}
}

func TestResolve_BareRepoUsesDefaultBranch(t *testing.T) {
	p, _ := Parse("https://github.com/o/r")
	check := func(ref string) (string, bool, error) {
		if ref == "trunk" {
			return "sha", true, nil
		}
		return "", false, nil
	}
	def := func() (string, error) { return "trunk", nil }
	src, err := p.Resolve(check, nil, def)
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "trunk" || src.Path != "" {
		t.Fatalf("got ref=%q path=%q", src.Ref, src.Path)
	}
}

// noCheck fails the test if a per-ref probe is made where the listing suffices.
func noCheck(t *testing.T) RefChecker {
	return func(ref string) (string, bool, error) {
		t.Helper()
		t.Fatalf("check(%q) should not be called", ref)
		return "", false, nil
	}
}

func lister(refs map[string]string, complete bool, calls *int) RefLister {
	return func(prefix string) (map[string]string, bool, error) {
		*calls++
		return refs, complete, nil
	}
}

func TestResolve_ListedDeepURLNeedsNoProbe(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/main/skills/engineering/tdd")
	calls := 0
	src, err := p.Resolve(noCheck(t), lister(map[string]string{"main": "sha-main"}, true, &calls), noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || src.Ref != "main" || src.Path != "skills/engineering/tdd" || src.Commit != "sha-main" {
		t.Fatalf("calls=%d ref=%q path=%q commit=%q", calls, src.Ref, src.Path, src.Commit)
	}
}

func TestResolve_ListedPrefersLongest(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/feature/x/skills")
	calls := 0
	refs := map[string]string{"feature": "s1", "feature/x": "s2"}
	src, err := p.Resolve(noCheck(t), lister(refs, true, &calls), noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "feature/x" || src.Path != "skills" || src.Commit != "s2" {
		t.Fatalf("got ref=%q path=%q commit=%q", src.Ref, src.Path, src.Commit)
	}
}

func TestResolve_ListedAnnotatedTagIsChecked(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/v1.0/docs")
	calls := 0
	check := func(ref string) (string, bool, error) {
		if ref == "v1.0" {
			return "commit-sha", true, nil
		}
		return "", false, nil
	}
	src, err := p.Resolve(check, lister(map[string]string{"v1.0": ""}, true, &calls), noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "v1.0" || src.Commit != "commit-sha" {
		t.Fatalf("got ref=%q commit=%q", src.Ref, src.Commit)
	}
}

func TestResolve_ListedFallsBackToCommitSHA(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/abc1234/docs/guide")
	calls := 0
	var checked []string
	check := func(ref string) (string, bool, error) {
		checked = append(checked, ref)
		return "abc1234full", ref == "abc1234", nil
	}
	src, err := p.Resolve(check, lister(map[string]string{}, true, &calls), noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "abc1234" || src.Path != "docs/guide" || len(checked) != 1 {
		t.Fatalf("got ref=%q path=%q checked=%v", src.Ref, src.Path, checked)
	}
}

func TestResolve_IncompleteListingProbes(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/main/a/b")
	calls := 0
	check := func(ref string) (string, bool, error) { return "sha", ref == "main", nil }
	src, err := p.Resolve(check, lister(nil, false, &calls), noDefault(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "main" || src.Path != "a/b" {
		t.Fatalf("got ref=%q path=%q", src.Ref, src.Path)
	}
}

func TestResolve_SingleSegmentSkipsListing(t *testing.T) {
	p, _ := Parse("https://github.com/o/r/tree/main")
	calls := 0
	check := func(ref string) (string, bool, error) { return "sha", ref == "main", nil }
	if _, err := p.Resolve(check, lister(nil, true, &calls), noDefault(t)); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("listing called %d times for a single-segment ref", calls)
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += "/"
		}
		out += x
	}
	return out
}
