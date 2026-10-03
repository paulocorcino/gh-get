package fetch

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDo_StalledBodyFails(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		select { // stall until the client gives up
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	c := New("", nil)
	c.stall = 100 * time.Millisecond
	resp, err := c.do(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	start := time.Now()
	_, err = io.ReadAll(resp.Body)
	if !errors.Is(err, errStalled) {
		t.Fatalf("err = %v, want errStalled", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("stall was not detected promptly")
	}
}

func TestDo_SlowButSteadyBodySucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 5; i++ {
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(40 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := New("", nil)
	c.stall = 100 * time.Millisecond // shorter than the total, longer than each gap
	resp, err := c.do(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "xxxxx" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestCollectRefs_FiltersPrefixAndAnnotatedTags(t *testing.T) {
	heads := `[
		{"ref":"refs/heads/main","object":{"sha":"m","type":"commit"}},
		{"ref":"refs/heads/maintenance","object":{"sha":"x","type":"commit"}},
		{"ref":"refs/heads/main/next","object":{"sha":"n","type":"commit"}}
	]`
	tags := `[{"ref":"refs/tags/main/v1","object":{"sha":"tagobj","type":"tag"}}]`

	refs := map[string]string{}
	if err := collectRefs([]byte(heads), "heads", "main", refs); err != nil {
		t.Fatal(err)
	}
	if err := collectRefs([]byte(tags), "tags", "main", refs); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"main": "m", "main/next": "n", "main/v1": ""}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
	for k, v := range want {
		if got, ok := refs[k]; !ok || got != v {
			t.Fatalf("refs[%q] = %q (present=%v), want %q", k, got, ok, v)
		}
	}
}
