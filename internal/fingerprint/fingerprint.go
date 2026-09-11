package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
)

type canonNode struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type canonEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type canonGraph struct {
	Nodes []canonNode `json:"nodes"`
	Edges []canonEdge `json:"edges"`
}

type canonAPI struct {
	Key        string   `json:"key"`
	Doc        string   `json:"doc,omitempty"`
	Signatures []string `json:"signatures"`
}

type canonicalPayload struct {
	Graph canonGraph `json:"graph"`
	APIs  []canonAPI `json:"apis"`
}

type Lockfile struct {
	Hash  string      `json:"hash"`
	Graph canonGraph  `json:"graph"`
	APIs  lang.APISet `json:"apis"`
}

func canonicalGraph(g graph.Graph) canonGraph {
	nodes := make([]canonNode, len(g.Nodes))
	for i, n := range g.Nodes {
		nodes[i] = canonNode{Key: n.Key, Label: n.Label}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Key < nodes[j].Key })

	edges := make([]canonEdge, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = canonEdge{From: e.From, To: e.To}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	return canonGraph{Nodes: nodes, Edges: edges}
}

func canonicalAPIs(apis lang.APISet) []canonAPI {
	out := make([]canonAPI, len(apis))
	for i, a := range apis {
		sigs := append([]string(nil), a.Signatures...)
		sort.Strings(sigs)
		out[i] = canonAPI{Key: a.Key, Doc: a.Doc, Signatures: sigs}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func sortedAPISet(apis lang.APISet) lang.APISet {
	canon := canonicalAPIs(apis)
	out := make(lang.APISet, len(canon))
	for i, a := range canon {
		out[i] = lang.PackageAPI{Key: a.Key, Doc: a.Doc, Signatures: a.Signatures}
	}
	return out
}

func Canonical(g graph.Graph, apis lang.APISet) ([]byte, error) {
	payload := canonicalPayload{
		Graph: canonicalGraph(g),
		APIs:  canonicalAPIs(apis),
	}
	return json.Marshal(payload)
}

func Hash(g graph.Graph, apis lang.APISet) (string, error) {
	b, err := Canonical(g, apis)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func NewLockfile(g graph.Graph, apis lang.APISet) (Lockfile, error) {
	h, err := Hash(g, apis)
	if err != nil {
		return Lockfile{}, err
	}
	return Lockfile{
		Hash:  h,
		Graph: canonicalGraph(g),
		APIs:  sortedAPISet(apis),
	}, nil
}

type lockfileJSON struct {
	Hash  string     `json:"hash"`
	Graph canonGraph `json:"graph"`
	APIs  []canonAPI `json:"apis"`
}

func (lf Lockfile) marshalJSON() ([]byte, error) {
	apis := canonicalAPIs(lf.APIs)
	return json.Marshal(lockfileJSON{
		Hash:  lf.Hash,
		Graph: lf.Graph,
		APIs:  apis,
	})
}

func Load(path string) (Lockfile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Lockfile{}, err
	}
	var raw lockfileJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return Lockfile{}, err
	}
	apis := make(lang.APISet, len(raw.APIs))
	for i, a := range raw.APIs {
		apis[i] = lang.PackageAPI{Key: a.Key, Doc: a.Doc, Signatures: a.Signatures}
	}
	return Lockfile{Hash: raw.Hash, Graph: raw.Graph, APIs: apis}, nil
}

func Write(path string, lf Lockfile) error {
	b, err := lf.marshalJSON()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}
