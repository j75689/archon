package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/j75689/archon/internal/lang"
)

var (
	ErrNotRepo = errors.New("not a git repository")
	ErrNoGit   = errors.New("git executable not found on PATH")
	ErrNoTag   = errors.New("no reachable git tag; pass --from or create a tag")
)

type Repo struct {
	Root string
	Bin  string
}

func Open(startDir string) (*Repo, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, ErrNoGit
	}

	r := &Repo{Bin: bin}
	out, err := r.cmd(startDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotRepo, err)
	}
	r.Root = strings.TrimSpace(string(out))
	return r, nil
}

func (r *Repo) Verify(rev string) (string, error) {
	out, err := r.cmd(r.Root, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("revision %q not found locally (no fetch): %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Repo) Snapshot(rev string, want func(string) bool) (lang.Snapshot, error) {
	resolved, err := r.Verify(rev)
	if err != nil {
		return lang.Snapshot{}, err
	}

	out, err := r.cmd(r.Root, "ls-tree", "-rz", "--name-only", resolved)
	if err != nil {
		return lang.Snapshot{}, err
	}

	files := map[string][]byte{}
	for _, path := range splitNUL(out) {
		if path == "" || (want != nil && !want(path)) {
			continue
		}

		body, err := r.cmd(r.Root, "show", resolved+":"+path)
		if err != nil {
			return lang.Snapshot{}, err
		}
		files[path] = body
	}

	return lang.Snapshot{Rev: resolved, Files: files}, nil
}

func (r *Repo) LogSubjects(from, to string, maxN, maxBytes int) ([]string, error) {
	if maxN <= 0 || maxBytes <= 0 {
		return nil, nil
	}

	rangeSpec := to
	if from != "" {
		rangeSpec = from + ".." + to
	}

	out, err := r.cmd(r.Root, "log", "--format=%s", rangeSpec)
	if err != nil {
		return nil, err
	}

	var subs []string
	used := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(subs) >= maxN || used+len(line)+1 > maxBytes {
			break
		}
		subs = append(subs, line)
		used += len(line) + 1
	}

	return subs, nil
}

type FromResult struct {
	From      string
	EmptyDiff bool
}

func (r *Repo) ResolveFrom(to, explicit string) (FromResult, error) {
	if explicit != "" {
		if _, err := r.Verify(explicit); err != nil {
			return FromResult{}, err
		}
		return FromResult{From: explicit}, nil
	}
	tag, err := r.DescribeTag(to)
	if err != nil {
		return FromResult{}, err
	}
	toHash, err := r.Verify(to)
	if err != nil {
		return FromResult{}, err
	}
	tagHash, err := r.Verify(tag)
	if err != nil {
		return FromResult{}, err
	}
	if toHash != tagHash {
		return FromResult{From: tag}, nil
	}
	parent := to + "^"
	prev, err := r.DescribeTag(parent)
	if err != nil {
		return FromResult{EmptyDiff: true}, nil
	}
	return FromResult{From: prev}, nil
}

func (r *Repo) DescribeTag(rev string) (string, error) {
	out, err := r.cmd(r.Root, "describe", "--tags", "--abbrev=0", rev)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoTag, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Repo) cmd(dir string, args ...string) ([]byte, error) {
	if dir == "" {
		dir = r.Root
	}

	cmd := exec.Command(r.Bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}

	return out, nil
}

func splitNUL(b []byte) []string {
	parts := bytes.Split(b, []byte{0})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, string(part))
	}
	return out
}
