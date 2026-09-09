package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	defaultTo      = "HEAD"
	defaultDoc     = "docs/ARCHITECTURE.md"
	defaultAnchor  = "data-flow"
	defaultModel   = "gpt-4o-mini"
	defaultBaseURL = "https://api.openai.com/v1"
)

type Values struct {
	From    string
	To      string
	Doc     string
	Anchor  string
	Model   string
	BaseURL string
	APIKey  string
	LLM     bool
}

type fileConfig struct {
	Doc     string `yaml:"doc"`
	Anchor  string `yaml:"anchor"`
	Model   string `yaml:"model"`
	BaseURL string `yaml:"base_url"`
}

func Load(repoRoot string, flags Values, getenv func(string) string, readFile func(string) ([]byte, error), home string) (Values, error) {
	repoCfg, err := loadYAML(filepath.Join(repoRoot, "archon.yaml"), readFile)
	if err != nil {
		return Values{}, err
	}

	userCfg, err := loadYAML(filepath.Join(home, ".config", "archon", "config.yaml"), readFile)
	if err != nil {
		return Values{}, err
	}

	envDoc := getenv("ARCHON_DOC")
	envModel := getenv("ARCHON_MODEL")
	envBaseURL := getenv("ARCHON_BASE_URL")
	envAPIKey := getenv("OPENAI_API_KEY")

	cfg := Values{
		From:    firstNonEmpty(flags.From),
		To:      firstNonEmpty(flags.To, defaultTo),
		Doc:     firstNonEmpty(flags.Doc, envDoc, repoCfg.Doc, userCfg.Doc, defaultDoc),
		Anchor:  firstNonEmpty(flags.Anchor, repoCfg.Anchor, userCfg.Anchor, defaultAnchor),
		Model:   firstNonEmpty(flags.Model, envModel, repoCfg.Model, userCfg.Model, defaultModel),
		BaseURL: firstNonEmpty(flags.BaseURL, envBaseURL, repoCfg.BaseURL, userCfg.BaseURL, defaultBaseURL),
		APIKey:  firstNonEmpty(flags.APIKey, envAPIKey),
	}

	explicitBaseURL := flags.BaseURL != "" || envBaseURL != "" || repoCfg.BaseURL != "" || userCfg.BaseURL != ""
	cfg.LLM = cfg.APIKey != "" || explicitBaseURL

	return cfg, nil
}

func loadYAML(path string, readFile func(string) ([]byte, error)) (fileConfig, error) {
	body, err := readFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileConfig{}, nil
		}
		return fileConfig{}, err
	}

	var cfg fileConfig
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return fileConfig{}, err
	}
	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
