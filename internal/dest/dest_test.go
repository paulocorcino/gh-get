package dest

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolve(t *testing.T) {
	// Build absolute paths rooted at a platform-appropriate base.
	base := "/work"
	if runtime.GOOS == "windows" {
		base = `C:\work`
	}
	cwd := filepath.Join(base, "project")

	cases := []struct {
		name   string
		dest   string
		exists bool
		force  bool
		want   Action
	}{
		{"dot resolves to cwd", cwd, true, false, WriteInPlace},
		{"fresh non-existent dest", filepath.Join(base, "out"), false, false, Replace},
		{"exists without force is refused", filepath.Join(base, "out"), true, false, Refuse},
		{"exists with force replaces", filepath.Join(base, "out"), true, true, Replace},
		{"force but dest is ancestor of cwd is refused", base, true, true, Refuse},
		{"force but dest equals cwd is in-place", cwd, true, true, WriteInPlace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.dest, cwd, tc.exists, tc.force)
			if got.Action != tc.want {
				t.Fatalf("Resolve(%q, %q, exists=%v, force=%v).Action = %v, want %v",
					tc.dest, cwd, tc.exists, tc.force, got.Action, tc.want)
			}
			if got.Action == Refuse && got.Reason == "" {
				t.Fatal("Refuse must carry a Reason")
			}
		})
	}
}

func TestResolveCaseInsensitiveCwdOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive cwd matching only applies on Windows")
	}
	cwd := `C:\Work\Project`
	if got := Resolve(`C:\work\project`, cwd, true, false); got.Action != WriteInPlace {
		t.Fatalf("expected WriteInPlace for case-differing cwd, got %v", got.Action)
	}
}
