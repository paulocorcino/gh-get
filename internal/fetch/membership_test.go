package fetch

import "testing"

func TestFolderMember(t *testing.T) {
	cases := []struct {
		name    string
		folder  string
		entry   string
		wantRel string
		wantOk  bool
	}{
		{"whole repo keeps everything", "", "a/b/c.txt", "a/b/c.txt", true},
		{"whole repo keeps root file", "", "README.md", "README.md", true},
		{"inside folder", "skills/handoff", "skills/handoff/x.md", "x.md", true},
		{"nested inside folder", "skills/handoff", "skills/handoff/sub/y.md", "sub/y.md", true},
		{"outside folder", "skills/handoff", "skills/other/z.md", "", false},
		{"sibling prefix is not a member", "skills/hand", "skills/handoff/x.md", "", false},
		{"exact folder is not a member", "skills/handoff", "skills/handoff", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := folderMember(tc.folder, tc.entry)
			if ok != tc.wantOk || rel != tc.wantRel {
				t.Fatalf("folderMember(%q, %q) = (%q, %v), want (%q, %v)",
					tc.folder, tc.entry, rel, ok, tc.wantRel, tc.wantOk)
			}
		})
	}
}
