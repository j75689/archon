package prompt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/config"
)

func TestRenderSubstitutesData(t *testing.T) {
	got, err := Render(
		"graph={{.Graph}}\napis={{.APIs}}\ndiff={{.Diff}}",
		Data{Graph: "graph text", APIs: "API text", Diff: "diff text"},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"graph=graph text", "apis=API text", "diff=diff text"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render output %q does not contain %q", got, want)
		}
	}
}

func TestLoadMissingPromptFileReturnsError(t *testing.T) {
	repoRoot := "/repo"
	promptPath := filepath.Join(repoRoot, "prompts", "architecture.txt")
	wantErr := errors.New("missing prompt")

	_, err := Load(
		repoRoot,
		config.Generator{ID: "architecture", Prompt: "prompts/architecture.txt"},
		func(path string) ([]byte, error) {
			if path != promptPath {
				t.Fatalf("read path = %q, want %q", path, promptPath)
			}
			return nil, wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Load error = %v, want error wrapping %v", err, wantErr)
	}
}

func TestLoadUsesBuiltInArchitecturePrompt(t *testing.T) {
	tmpl, err := Load(
		"/repo",
		config.Generator{ID: "architecture"},
		func(string) ([]byte, error) {
			return nil, os.ErrNotExist
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Render(tmpl, Data{
		Graph: "architecture graph",
		APIs:  "exported API",
		Diff:  "structure diff",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"architecture graph",
		"exported API",
		"structure diff",
		"System Graph",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered built-in prompt %q does not contain %q", got, want)
		}
	}
}

func TestBuiltInSupportsConfiguredGeneratorIDs(t *testing.T) {
	for _, id := range []string{"architecture", "workflow", "packages"} {
		if _, ok := BuiltIn(id); !ok {
			t.Errorf("BuiltIn(%q) not found", id)
		}
	}

	if _, ok := BuiltIn("unknown"); ok {
		t.Fatal(`BuiltIn("unknown") found, want missing`)
	}
}

func TestLoadUnknownGeneratorIDWithoutPromptReturnsError(t *testing.T) {
	_, err := Load("/repo", config.Generator{ID: "adr"}, func(string) ([]byte, error) {
		t.Fatal("readFile must not be called")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `no built-in prompt for generator "adr"`) {
		t.Fatalf("error %v", err)
	}
}
