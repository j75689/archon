package log

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriterLevel1InfoOnly(t *testing.T) {
	var buf bytes.Buffer
	w := Writer{W: &buf, Level: 1}
	w.Info("snapshot HEAD: 2 files")
	w.Debug("snapshot: a.go")
	if buf.String() != "snapshot HEAD: 2 files\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestWriterLevel2InfoAndDebug(t *testing.T) {
	var buf bytes.Buffer
	w := Writer{W: &buf, Level: 2}
	w.Info("extract go: 1 packages, 0 edges")
	w.Debug("parse: a.go")
	if buf.String() != "extract go: 1 packages, 0 edges\nparse: a.go\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestWriterLevel0Silent(t *testing.T) {
	var buf bytes.Buffer
	w := Writer{W: &buf, Level: 0}
	w.Info("x")
	w.Debug("y")
	if buf.Len() != 0 {
		t.Fatalf("got %q", buf.String())
	}
}

func TestNopAndNilSilent(t *testing.T) {
	Nop{}.Info("x")
	Nop{}.Debug("y")
	OrNop(nil).Info("x")
	OrNop(nil).Debug("y")
}

func TestWriterNilWDoesNotPanic(t *testing.T) {
	Writer{Level: 2}.Info("x")
	Writer{Level: 2}.Debug("y")
}

func TestFromVerbose(t *testing.T) {
	if _, ok := FromVerbose(nil, 0).(Nop); !ok {
		t.Fatal("level 0 must be Nop")
	}
	var buf bytes.Buffer
	l := FromVerbose(&buf, 3)
	w, ok := l.(Writer)
	if !ok || w.Level != 2 {
		t.Fatalf("level 3 must cap at 2: %#v", l)
	}
	l.Debug("snapshot: a.go")
	if !strings.Contains(buf.String(), "snapshot: a.go") {
		t.Fatalf("got %q", buf.String())
	}
}
