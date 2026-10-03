package manifest

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse_EntriesCommentsAndDestinations(t *testing.T) {
	in := "\xef\xbb\xbf# skills I use\n" +
		"\n" +
		"https://github.com/o/r/tree/main/skills/a\n" +
		"  https://github.com/o/r/tree/main/skills/b   vendor/my b  # pinned later\n" +
		"https://github.com/o/whole\t./whole-repo\n"

	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Line: 3, URL: "https://github.com/o/r/tree/main/skills/a"},
		{Line: 4, URL: "https://github.com/o/r/tree/main/skills/b", Dest: "vendor/my b"},
		{Line: 5, URL: "https://github.com/o/whole", Dest: "./whole-repo"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestParse_KeepsHashInsideURL(t *testing.T) {
	got, err := Parse(strings.NewReader("https://github.com/o/r/tree/main/a#readme\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://github.com/o/r/tree/main/a#readme" {
		t.Fatalf("got %#v", got)
	}
}

func TestParse_InvalidURLReportsLine(t *testing.T) {
	_, err := Parse(strings.NewReader("# ok\nhttps://gitlab.com/o/r\n"))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestParse_Empty(t *testing.T) {
	got, err := Parse(strings.NewReader("# nothing\n\n"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %#v, err %v", got, err)
	}
}
