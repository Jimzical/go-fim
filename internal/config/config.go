package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the runtime config after parsing + regex compilation.
type Config struct {
	Path               string
	Exclude            []*regexp.Regexp
	DBPath             string
	HistoryDir         string
	ServerURL          string // empty = standalone mode (no POST)
	AgentName          string // operator-chosen display label, sent on every /report
	AgentID            string
	InsecureSkipVerify bool // disable TLS certificate verification entirely
}

// rawConfig is the on-disk YAML format; Load translates it into Config
type rawConfig struct {
	Path               string   `yaml:"path"`
	Exclude            []string `yaml:"exclude"`
	DBPath             string   `yaml:"db_path"`
	ServerURL          string   `yaml:"server_url"`
	AgentName          string   `yaml:"agent_name"`
	AgentID            string   `yaml:"agent_id"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"`
}

func Default() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("default config: %w", err)
	}
	return configFromRoot(cwd), nil
}

func configFromRoot(root string) *Config {
	gofim := filepath.Join(root, ".gofim")
	return &Config{
		Path:       root,
		DBPath:     filepath.Join(gofim, "snapshot.db"),
		HistoryDir: filepath.Join(gofim, "history"),
	}
}

// Load reads, parses, and validates the YAML config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if raw.Path == "" {
		return nil, fmt.Errorf("config %q: 'path' is required", path)
	}
	resolvedRoot, err := resolvePath(raw.Path)
	if err != nil {
		return nil, fmt.Errorf("config %q: path %q: %w", path, raw.Path, err)
	}

	cfg := configFromRoot(resolvedRoot)
	if raw.DBPath != "" {
		resolved, err := resolvePath(raw.DBPath)
		if err != nil {
			return nil, fmt.Errorf("config %q: db_path %q: %w", path, raw.DBPath, err)
		}
		cfg.DBPath = resolved
		cfg.HistoryDir = filepath.Join(filepath.Dir(resolved), "history")
	}
	cfg.ServerURL = raw.ServerURL
	cfg.AgentName = raw.AgentName
	cfg.AgentID = raw.AgentID
	cfg.InsecureSkipVerify = raw.InsecureSkipVerify

	for i, pat := range raw.Exclude {
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("config %q: exclude[%d] %q: %w", path, i, pat, err)
		}
		cfg.Exclude = append(cfg.Exclude, re)
	}
	return cfg, nil
}

// resolvePath expands a leading ~ to the user's home directory, then converts
// the result to an absolute, cleaned path. The shell normally does ~-expansion,
// but values read from YAML never see a shell.
func resolvePath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve ~: %w", err)
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}
	return filepath.Abs(p)
}
