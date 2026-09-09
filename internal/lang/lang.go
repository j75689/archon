package lang

import "github.com/j75689/archon/internal/graph"

type Snapshot struct {
	Rev   string
	Files map[string][]byte
}

type Extractor interface {
	Name() string
	Match(Snapshot) bool
	Extract(Snapshot) (graph.Graph, error)
}
