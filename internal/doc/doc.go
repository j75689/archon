package doc

import (
	"bytes"
	"fmt"
	"strings"
)

func startLine(id string) string { return "<!-- ARCHON:START:" + id + " -->" }

func endLine(id string) string { return "<!-- ARCHON:END:" + id + " -->" }

func ExtractRegion(src []byte, id string) (region string, ok bool, err error) {
	lines := bytes.Split(src, []byte("\n"))
	start, end := -1, -1
	for i, line := range lines {
		t := strings.TrimSpace(string(bytes.TrimSuffix(line, []byte("\r"))))
		switch t {
		case startLine(id):
			if start != -1 {
				return "", false, fmt.Errorf("nested or duplicate START for %s", id)
			}
			start = i
		case endLine(id):
			if start == -1 {
				return "", false, fmt.Errorf("END without START for %s", id)
			}
			if end != -1 {
				return "", false, fmt.Errorf("duplicate END for %s", id)
			}
			end = i
		}
	}

	if start == -1 && end == -1 {
		return "", false, nil
	}
	if start == -1 || end == -1 || end < start {
		return "", false, fmt.Errorf("invalid archon anchors for %s", id)
	}

	var b strings.Builder
	for i := start + 1; i < end; i++ {
		b.Write(bytes.TrimSuffix(lines[i], []byte("\r")))
		if i+1 < end {
			b.WriteByte('\n')
		}
	}
	return b.String(), true, nil
}

func NormalizeNL(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
