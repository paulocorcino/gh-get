package fetch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paulocorcino/gh-get/internal/ghurl"
)

func TestFile_ReadsRawContentAtCommit(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		write(t, w, []byte("https://github.com/o/r/tree/main/x\n"))
	}))
	defer srv.Close()
	old := rawBase
	rawBase = srv.URL
	defer func() { rawBase = old }()

	src := ghurl.Build("o", "r", "main", "lists/my list.txt", "abc123")
	data, err := New("", nil).File(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "https://github.com/o/r/tree/main/x\n" {
		t.Fatalf("data = %q", data)
	}
	if gotPath != "/o/r/abc123/lists/my list.txt" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestFile_MissingFileFails(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	old := rawBase
	rawBase = srv.URL
	defer func() { rawBase = old }()

	if _, err := New("", nil).File(ghurl.Build("o", "r", "main", "nope.txt", "abc")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}
