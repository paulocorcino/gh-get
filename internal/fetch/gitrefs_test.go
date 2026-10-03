package fetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

const (
	shaMain = "1111111111111111111111111111111111111111"
	shaFeat = "2222222222222222222222222222222222222222"
	shaTag  = "3333333333333333333333333333333333333333" // tag object
	shaTagC = "4444444444444444444444444444444444444444" // commit it points to
)

func pkt(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }

// advertisementBody mimics GitHub's info/refs?service=git-upload-pack reply.
func advertisementBody() string {
	return pkt("# service=git-upload-pack\n") + "0000" +
		pkt(shaMain+" HEAD\x00multi_ack symref=HEAD:refs/heads/main agent=git/x\n") +
		pkt(shaFeat+" refs/heads/feature/x\n") +
		pkt(shaMain+" refs/heads/main\n") +
		pkt(shaMain+" refs/heads/maintenance\n") +
		pkt(shaTag+" refs/tags/v1\n") +
		pkt(shaTagC+" refs/tags/v1^{}\n") +
		"0000"
}

func TestParseAdvertisement(t *testing.T) {
	adv, err := parseAdvertisement([]byte(advertisementBody()))
	if err != nil {
		t.Fatal(err)
	}
	if adv.head != "main" {
		t.Fatalf("head = %q, want main", adv.head)
	}
	for ref, want := range map[string]string{"main": shaMain, "feature/x": shaFeat, "v1": shaTagC, "abc1234": "abc1234"} {
		if got, ok := adv.resolve(ref); !ok || got != want {
			t.Fatalf("resolve(%q) = %q, %v; want %q", ref, got, ok, want)
		}
	}
	if _, ok := adv.resolve("nope"); ok {
		t.Fatal("unknown ref resolved")
	}
	m := adv.matching("main")
	if len(m) != 1 || m["main"] != shaMain {
		t.Fatalf("matching(main) = %v", m)
	}
}

func TestParseAdvertisement_EmptyRepo(t *testing.T) {
	body := pkt("# service=git-upload-pack\n") + "0000" +
		pkt(strings.Repeat("0", 40)+" capabilities^{}\x00agent=git/x\n") + "0000"
	adv, err := parseAdvertisement([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(adv.refs) != 0 || adv.head != "" {
		t.Fatalf("adv = %+v, want empty", adv)
	}
}

func tarballOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// TestRateLimitedRunAvoidsAPI drives a whole download while the API answers
// every request with 403: refs must come from the git endpoint and content
// from codeload, with a single warning.
func TestRateLimitedRunAvoidsAPI(t *testing.T) {
	tarball := tarballOf(t, map[string]string{
		"r-" + shaFeat + "/docs/a.txt":  "alpha",
		"r-" + shaFeat + "/other/b.txt": "bravo",
	})
	apiCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case strings.HasPrefix(p, "/api/"):
			apiCalls++
			w.WriteHeader(http.StatusForbidden)
		case p == "/git/o/r.git/info/refs":
			w.Write([]byte(advertisementBody()))
		case p == "/codeload/o/r/tar.gz/"+shaFeat:
			w.Write(tarball)
		default:
			t.Errorf("unexpected request %s", p)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	for ptr, val := range map[*string]string{&apiBase: "/api", &gitBase: "/git", &codeloadBase: "/codeload"} {
		old := *ptr
		*ptr = srv.URL + val
		defer func() { *ptr = old }()
	}

	var warnings []string
	c := New("", func(s string) { warnings = append(warnings, s) })

	p, err := ghurl.Parse("https://github.com/o/r/tree/feature/x/docs")
	if err != nil {
		t.Fatal(err)
	}
	src, err := p.Resolve(c.CheckRef("o", "r"), c.ListRefs("o", "r"),
		func() (string, error) { return c.DefaultBranch("o", "r") })
	if err != nil {
		t.Fatal(err)
	}
	if src.Ref != "feature/x" || src.Path != "docs" || src.Commit != shaFeat {
		t.Fatalf("src = %+v", src)
	}

	dest := t.TempDir()
	if err := c.Materialize(src, dest, Replace, nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dest, "a.txt")); got != "alpha" {
		t.Fatalf("a.txt = %q", got)
	}

	if branch, err := c.DefaultBranch("o", "r"); err != nil || branch != "main" {
		t.Fatalf("DefaultBranch = %q, %v", branch, err)
	}
	if apiCalls != 1 {
		t.Fatalf("API was called %d times; only the first refusal is expected", apiCalls)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %q, want exactly one", warnings)
	}
}
