package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/exitcode"
)

func TestRunDiffNoRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff"}, &stdout, &stderr)
	if code != exitcode.Fail {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not a git repository") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
