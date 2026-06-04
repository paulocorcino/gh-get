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
	src, err := p.Resolve(check, noDefault(t))
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
	src, err := p.Resolve(check, noDefault(t))
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
	if _, err := p.Resolve(check, noDefault(t)); err == nil {
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
	src, err := p.Resolve(check, noDefault(t))
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
	src, err := p.Resolve(check, def)
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "trunk" || src.Path != "" {
		t.Fatalf("got ref=%q path=%q", src.Ref, src.Path)
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
