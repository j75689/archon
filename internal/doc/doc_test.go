package doc

import (
	"bytes"
	"strings"
	"testing"
)

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

func TestReplaceRegionPreservesHumanText(t *testing.T) {
	src := []byte("# Architecture\n\nHuman rationale stays.\n\n<!-- ARCHON:START:data-flow -->\n```mermaid\nold\n```\n<!-- ARCHON:END:data-flow -->\n")
	payload := "```mermaid\nflowchart LR\nA --> B\n```\n"

	got, err := ReplaceRegion(src, "data-flow", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("Human rationale stays.")) {
		t.Fatalf("human text lost: %s", got)
	}
	region, ok, err := ExtractRegion(got, "data-flow")
	if err != nil || !ok {
		t.Fatalf("extract after replace: ok=%v err=%v", ok, err)
	}
	if region != strings.TrimSuffix(payload, "\n") {
		t.Fatalf("region %q want %q", region, strings.TrimSuffix(payload, "\n"))
	}
}

func TestReplaceRegionMissingEND(t *testing.T) {
	src := []byte("# Title\n\n<!-- ARCHON:START:data-flow -->\nbody\n")
	_, err := ReplaceRegion(src, "data-flow", "new\n")
	if err == nil {
		t.Fatal("expected error for missing END")
	}
}

func TestReplaceRegionMissingAnchors(t *testing.T) {
	src := []byte("# Title\n\nNo anchors here.\n")
	_, err := ReplaceRegion(src, "data-flow", "new\n")
	if err == nil {
		t.Fatal("expected error for missing anchors")
	}
	if !strings.Contains(err.Error(), "missing archon anchors") {
		t.Fatalf("got %v", err)
	}
}

func TestReplaceRegionNestedSTART(t *testing.T) {
	src := []byte("<!-- ARCHON:START:data-flow -->\n<!-- ARCHON:START:data-flow -->\n<!-- ARCHON:END:data-flow -->\n")
	_, err := ReplaceRegion(src, "data-flow", "new\n")
	if err == nil {
		t.Fatal("expected nested START error")
	}
}

func TestReplaceRegionPreservesCRLFOutside(t *testing.T) {
	src := []byte("# Title\r\n\r\n<!-- ARCHON:START:data-flow -->\r\nold\r\n<!-- ARCHON:END:data-flow -->\r\n")
	got, err := ReplaceRegion(src, "data-flow", "new\n")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("# Title\r\n\r\n<!-- ARCHON:START:data-flow -->\r\n")) {
		t.Fatalf("prefix not preserved: %q", got[:min(50, len(got))])
	}
	if !bytes.HasSuffix(got, []byte("<!-- ARCHON:END:data-flow -->\r\n")) {
		t.Fatalf("suffix not preserved: %q", got[len(got)-40:])
	}
	region, ok, err := ExtractRegion(got, "data-flow")
	if err != nil || !ok || region != "new" {
		t.Fatalf("region %q ok=%v err=%v", region, ok, err)
	}
}

func TestNewDocument(t *testing.T) {
	payload := "```mermaid\nflowchart LR\n```\n"
	got := NewDocument(payload, "data-flow")
	want := "# Architecture\n\n<!-- ARCHON:START:data-flow -->\n" + payload + "<!-- ARCHON:END:data-flow -->\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAppendAnchor(t *testing.T) {
	src := []byte("# Existing doc\n\nHuman rationale stays.")
	payload := "```mermaid\nflowchart LR\n```\n"
	got := AppendAnchor(src, "data-flow", payload)
	if !bytes.Contains(got, []byte("Human rationale stays.")) {
		t.Fatal("existing content lost")
	}
	region, ok, err := ExtractRegion(got, "data-flow")
	if err != nil || !ok {
		t.Fatalf("extract: ok=%v err=%v", ok, err)
	}
	if region != strings.TrimSuffix(payload, "\n") {
		t.Fatalf("region %q", region)
	}
}

func TestAppendAnchorAlreadyNewline(t *testing.T) {
	src := []byte("# Doc\n")
	got := AppendAnchor(src, "x", "p\n")
	if !bytes.HasPrefix(got, []byte("# Doc\n")) {
		t.Fatalf("prefix %q", got[:10])
	}
}

func TestNewDocumentPayloadWithoutTrailingNewline(t *testing.T) {
	payload := "flowchart LR"
	got := NewDocument(payload, "data-flow")
	if !bytes.Contains(got, []byte("flowchart LR\n<!-- ARCHON:END:data-flow -->")) {
		t.Fatalf("END must be on its own line after payload: %q", got)
	}
	region, ok, err := ExtractRegion(got, "data-flow")
	if err != nil || !ok {
		t.Fatalf("extract: ok=%v err=%v", ok, err)
	}
	if region != payload {
		t.Fatalf("region %q want %q", region, payload)
	}
}

func TestAppendAnchorPayloadWithoutTrailingNewline(t *testing.T) {
	src := []byte("# Existing doc\n")
	payload := "flowchart LR"
	got := AppendAnchor(src, "data-flow", payload)
	if !bytes.Contains(got, []byte("flowchart LR\n<!-- ARCHON:END:data-flow -->")) {
		t.Fatalf("END must be on its own line after payload: %q", got)
	}
	region, ok, err := ExtractRegion(got, "data-flow")
	if err != nil || !ok {
		t.Fatalf("extract: ok=%v err=%v", ok, err)
	}
	if region != payload {
		t.Fatalf("region %q want %q", region, payload)
	}
}
