package doc

import "testing"

func TestExtractRegion(t *testing.T) {
	src := []byte("# Title\r\n\r\n<!-- ARCHON:START:data-flow -->\r\nline 1\r\nline 2\r\n<!-- ARCHON:END:data-flow -->\r\n")

	got, ok, err := ExtractRegion(src, "data-flow")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected anchored region")
	}
	if got != "line 1\nline 2" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractRegionMissing(t *testing.T) {
	got, ok, err := ExtractRegion([]byte("# Title\n"), "data-flow")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected missing region")
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractRegionInvalidAnchors(t *testing.T) {
	_, _, err := ExtractRegion([]byte("<!-- ARCHON:END:data-flow -->\n"), "data-flow")
	if err == nil {
		t.Fatal("expected invalid anchors error")
	}
}

func TestNormalizeNL(t *testing.T) {
	if got := NormalizeNL("a\r\nb\r\n"); got != "a\nb\n" {
		t.Fatalf("got %q", got)
	}
}
