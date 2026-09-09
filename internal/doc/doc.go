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

type anchorSpan struct {
	startContentOff int
	endLineStart    int
}

func locateAnchors(src []byte, id string) (anchorSpan, bool, error) {
	startLineIdx, endLineIdx := -1, -1
	var span anchorSpan

	off := 0
	lineNum := 0
	for {
		lineStart := off
		lineEnd := off
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}

		line := src[lineStart:lineEnd]
		trimmed := strings.TrimSpace(string(bytes.TrimSuffix(line, []byte("\r"))))
		switch trimmed {
		case startLine(id):
			if startLineIdx != -1 {
				return anchorSpan{}, false, fmt.Errorf("nested or duplicate START for %s", id)
			}
			startLineIdx = lineNum
			if lineEnd < len(src) {
				span.startContentOff = lineEnd + 1
			} else {
				span.startContentOff = lineEnd
			}
		case endLine(id):
			if startLineIdx == -1 {
				return anchorSpan{}, false, fmt.Errorf("END without START for %s", id)
			}
			if endLineIdx != -1 {
				return anchorSpan{}, false, fmt.Errorf("duplicate END for %s", id)
			}
			endLineIdx = lineNum
			span.endLineStart = lineStart
		}

		if lineEnd >= len(src) {
			break
		}
		off = lineEnd + 1
		lineNum++
	}

	if startLineIdx == -1 && endLineIdx == -1 {
		return anchorSpan{}, false, nil
	}
	if startLineIdx == -1 || endLineIdx == -1 || endLineIdx < startLineIdx {
		return anchorSpan{}, false, fmt.Errorf("invalid archon anchors for %s", id)
	}
	return span, true, nil
}

func ReplaceRegion(src []byte, id, payload string) ([]byte, error) {
	span, ok, err := locateAnchors(src, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing archon anchors")
	}

	payload = NormalizeNL(payload)
	if payload == "" || !strings.HasSuffix(payload, "\n") {
		payload += "\n"
	}

	var out bytes.Buffer
	out.Write(src[:span.startContentOff])
	out.WriteString(payload)
	out.Write(src[span.endLineStart:])
	return out.Bytes(), nil
}

func NewDocument(payload, id string) []byte {
	payload = NormalizeNL(payload)
	var b strings.Builder
	b.WriteString("# Architecture\n\n")
	b.WriteString(startLine(id))
	b.WriteByte('\n')
	b.WriteString(payload)
	b.WriteString(endLine(id))
	b.WriteByte('\n')
	return []byte(b.String())
}

func AppendAnchor(src []byte, id, payload string) []byte {
	payload = NormalizeNL(payload)
	var b bytes.Buffer
	b.Write(src)
	if len(src) > 0 && src[len(src)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(startLine(id))
	b.WriteByte('\n')
	b.WriteString(payload)
	b.WriteString(endLine(id))
	b.WriteByte('\n')
	return b.Bytes()
}
