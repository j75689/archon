package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/j75689/archon/internal/app"
	"github.com/j75689/archon/internal/config"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/git"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	root := newRoot(stdout, stderr)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		var exitErr errExit
		if errors.As(err, &exitErr) {
			return exitErr.code
		}
		fmt.Fprintln(stderr, err)
		return exitcode.Fail
	}

	return exitcode.OK
}

func newRoot(stdout, stderr io.Writer) *cobra.Command {
	var from string
	var to string
	var doc string

	root := &cobra.Command{
		Use:           "archon",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.PersistentFlags().StringVar(&from, "from", "", "git revision for the left side of a drift comparison")
	root.PersistentFlags().StringVar(&to, "to", "HEAD", "git revision to inspect")
	root.PersistentFlags().StringVar(&doc, "doc", "", "architecture markdown path relative to repo root")

	newApp := func() (*app.App, error) {
		repo, err := git.Open(".")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return nil, errExit{code: exitcode.Fail}
		}
		home, _ := os.UserHomeDir()
		cfg, err := config.Load(
			repo.Root,
			config.Values{
				From: from,
				To:   to,
				Doc:  doc,
			},
			os.Getenv,
			os.ReadFile,
			home,
		)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return nil, errExit{code: exitcode.Fail}
		}

		a := app.New(repo)
		a.Stdout = stdout
		a.Stderr = stderr
		a.From = cfg.From
		a.To = cfg.To
		a.Doc = cfg.Doc
		a.Anchor = cfg.Anchor
		a.Model = cfg.Model
		a.BaseURL = cfg.BaseURL
		a.APIKey = cfg.APIKey
		a.LLM = cfg.LLM
		return a, nil
	}

	root.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Fail if the architecture doc does not match the graph at --to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return exitCodeErr(a.Check())
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "diff",
		Short: "Fail if the first-party graph changed between --from and --to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return exitCodeErr(a.Diff())
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Write the graph at --to into the architecture doc",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return exitCodeErr(a.Sync())
		},
	})

	return root
}

func exitCodeErr(code int) error {
	if code == exitcode.OK {
		return nil
	}
	return errExit{code: code}
}

type errExit struct {
	code int
}

func (e errExit) Error() string {
	return fmt.Sprintf("exit %d", e.code)
}
