//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/j75689/archon/internal/fingerprint"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang/golang"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	r, err := git.Open(".")
	if err != nil {
		return err
	}
	snap, err := r.Snapshot("HEAD", golang.WantFile)
	if err != nil {
		return err
	}
	var ext golang.Extractor
	g, err := ext.Extract(snap)
	if err != nil {
		return err
	}
	g, err = graph.Compile(g)
	if err != nil {
		return err
	}
	apis, err := ext.ExtractAPIs(snap)
	if err != nil {
		return err
	}
	lf, err := fingerprint.NewLockfile(g, apis)
	if err != nil {
		return err
	}
	return fingerprint.Write(".archon/graph.json", lf)
}
