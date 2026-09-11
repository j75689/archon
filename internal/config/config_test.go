package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadPrefersFlagDocOverEnvAndYAML(t *testing.T) {
	repoRoot := "/repo"
	home := "/home/test"

	got, err := Load(
		repoRoot,
		Values{Doc: "docs/FLAG.md"},
		func(key string) string {
			if key == "ARCHON_DOC" {
				return "docs/ENV.md"
			}
			return ""
		},
		fakeReadFile(map[string]string{
			filepath.Join(repoRoot, "archon.yaml"):            "doc: docs/REPO.md\n",
			filepath.Join(home, ".config/archon/config.yaml"): "doc: docs/USER.md\n",
		}),
		home,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.Doc != "docs/FLAG.md" {
		t.Fatalf("Doc = %q, want %q", got.Doc, "docs/FLAG.md")
	}
}

func TestLoadUsesEnvDocWhenFlagEmpty(t *testing.T) {
	repoRoot := "/repo"
	home := "/home/test"

	got, err := Load(
		repoRoot,
		Values{},
		func(key string) string {
			if key == "ARCHON_DOC" {
				return "docs/ENV.md"
			}
			return ""
		},
		fakeReadFile(nil),
		home,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.Doc != "docs/ENV.md" {
		t.Fatalf("Doc = %q, want %q", got.Doc, "docs/ENV.md")
	}
}

func TestLoadPrefersRepoYAMLOverUserConfig(t *testing.T) {
	repoRoot := "/repo"
	home := "/home/test"

	got, err := Load(
		repoRoot,
		Values{},
		func(string) string { return "" },
		fakeReadFile(map[string]string{
			filepath.Join(repoRoot, "archon.yaml"):            "doc: docs/REPO.md\nanchor: repo-anchor\nmodel: repo-model\nbase_url: http://repo.example/v1\n",
			filepath.Join(home, ".config/archon/config.yaml"): "doc: docs/USER.md\nanchor: user-anchor\nmodel: user-model\nbase_url: http://user.example/v1\n",
		}),
		home,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.Doc != "docs/REPO.md" {
		t.Fatalf("Doc = %q, want %q", got.Doc, "docs/REPO.md")
	}
	if got.Anchor != "repo-anchor" {
		t.Fatalf("Anchor = %q, want %q", got.Anchor, "repo-anchor")
	}
	if got.Model != "repo-model" {
		t.Fatalf("Model = %q, want %q", got.Model, "repo-model")
	}
	if got.BaseURL != "http://repo.example/v1" {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, "http://repo.example/v1")
	}
	if !got.LLM {
		t.Fatalf("LLM = %v, want true", got.LLM)
	}
}

func TestLoadAppliesDefaultsAndLeavesLLMDisabled(t *testing.T) {
	got, err := Load(
		"/repo",
		Values{},
		func(string) string { return "" },
		fakeReadFile(nil),
		"/home/test",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.To != "HEAD" {
		t.Fatalf("To = %q, want %q", got.To, "HEAD")
	}
	if got.Doc != "docs/ARCHITECTURE.md" {
		t.Fatalf("Doc = %q, want %q", got.Doc, "docs/ARCHITECTURE.md")
	}
	if got.Anchor != "data-flow" {
		t.Fatalf("Anchor = %q, want %q", got.Anchor, "data-flow")
	}
	if got.Model != "gpt-4o-mini" {
		t.Fatalf("Model = %q, want %q", got.Model, "gpt-4o-mini")
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, "https://api.openai.com/v1")
	}
	if got.LLM {
		t.Fatalf("LLM = %v, want false", got.LLM)
	}
}

func TestLoadEnablesLLMForEnvBaseURLWithoutAPIKey(t *testing.T) {
	got, err := Load(
		"/repo",
		Values{},
		func(key string) string {
			if key == "ARCHON_BASE_URL" {
				return "http://localhost:11434/v1"
			}
			return ""
		},
		fakeReadFile(nil),
		"/home/test",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, "http://localhost:11434/v1")
	}
	if !got.LLM {
		t.Fatalf("LLM = %v, want true", got.LLM)
	}
}

func TestLoadSkipsUserYAMLWhenHomeEmpty(t *testing.T) {
	repoRoot := "/repo"
	userConfigPath := filepath.Join(".config", "archon", "config.yaml")

	got, err := Load(
		repoRoot,
		Values{},
		func(string) string { return "" },
		func(path string) ([]byte, error) {
			if path == userConfigPath {
				t.Fatalf("readFile called with user config path %q when home is empty", path)
			}
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.To != "HEAD" {
		t.Fatalf("To = %q, want %q", got.To, "HEAD")
	}
	if got.Doc != "docs/ARCHITECTURE.md" {
		t.Fatalf("Doc = %q, want %q", got.Doc, "docs/ARCHITECTURE.md")
	}
	if got.Anchor != "data-flow" {
		t.Fatalf("Anchor = %q, want %q", got.Anchor, "data-flow")
	}
	if got.Model != "gpt-4o-mini" {
		t.Fatalf("Model = %q, want %q", got.Model, "gpt-4o-mini")
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, "https://api.openai.com/v1")
	}
	if got.LLM {
		t.Fatalf("LLM = %v, want false", got.LLM)
	}
}

func TestLoadEnablesLLMForAPIKey(t *testing.T) {
	got, err := Load(
		"/repo",
		Values{},
		func(key string) string {
			if key == "OPENAI_API_KEY" {
				return "secret"
			}
			return ""
		},
		fakeReadFile(nil),
		"/home/test",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.APIKey != "secret" {
		t.Fatalf("APIKey = %q, want %q", got.APIKey, "secret")
	}
	if !got.LLM {
		t.Fatalf("LLM = %v, want true", got.LLM)
	}
}

func TestLoadUsesRepoGeneratorsWithoutConcatenatingUserGenerators(t *testing.T) {
	repoRoot := "/repo"
	home := "/home/test"
	want := []Generator{
		{ID: "architecture", Path: "docs/SYSTEM.md", Prompt: "prompts/system.txt"},
		{ID: "packages", Path: "docs/MODULES.md"},
	}

	got, err := Load(
		repoRoot,
		Values{Generators: []Generator{
			{ID: "flag", Path: "docs/FLAG.md"},
		}},
		func(string) string { return "" },
		fakeReadFile(map[string]string{
			filepath.Join(repoRoot, "archon.yaml"): `generators:
  - id: architecture
    path: docs/SYSTEM.md
    prompt: prompts/system.txt
  - id: packages
    path: docs/MODULES.md
`,
			filepath.Join(home, ".config/archon/config.yaml"): `generators:
  - id: workflow
    path: docs/USER-WORKFLOW.md
`,
		}),
		home,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got.Generators, want) {
		t.Fatalf("Generators = %#v, want %#v", got.Generators, want)
	}
}

func TestLoadUsesUserGeneratorsWhenRepoListEmpty(t *testing.T) {
	repoRoot := "/repo"
	home := "/home/test"
	want := []Generator{{ID: "workflow", Path: "docs/USER-WORKFLOW.md"}}

	got, err := Load(
		repoRoot,
		Values{},
		func(string) string { return "" },
		fakeReadFile(map[string]string{
			filepath.Join(repoRoot, "archon.yaml"): "generators: []\n",
			filepath.Join(home, ".config/archon/config.yaml"): `generators:
  - id: workflow
    path: docs/USER-WORKFLOW.md
`,
		}),
		home,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got.Generators, want) {
		t.Fatalf("Generators = %#v, want %#v", got.Generators, want)
	}
}

func TestLoadUsesDefaultGeneratorsWhenYAMLListsEmpty(t *testing.T) {
	got, err := Load(
		"/repo",
		Values{},
		func(string) string { return "" },
		fakeReadFile(map[string]string{
			filepath.Join("/repo", "archon.yaml"):                     "generators: []\n",
			filepath.Join("/home/test", ".config/archon/config.yaml"): "generators: []\n",
		}),
		"/home/test",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got.Generators, DefaultGenerators) {
		t.Fatalf("Generators = %#v, want %#v", got.Generators, DefaultGenerators)
	}
	if len(got.Generators) != 3 {
		t.Fatalf("len(Generators) = %d, want 3", len(got.Generators))
	}
}

func fakeReadFile(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		if body, ok := files[path]; ok {
			return []byte(body), nil
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
}
