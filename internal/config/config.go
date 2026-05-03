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
	Verbose            bool
	DBPath             string
	HistoryDir         string
	ServerURL          string // empty = standalone mode (no POST)
	AgentName          string // operator-chosen display label, sent on every /report
	InsecureSkipVerify bool   // disable TLS certificate verification entirely
}

// rawConfig matches the on-disk YAML shape; we translate it into Config
// (compiling regexes, resolving paths) so the rest of the program sees
// ready-to-use types.
type rawConfig struct {
	Path               string   `yaml:"path"`
	Exclude            []string `yaml:"exclude"`
	Verbose            bool     `yaml:"verbose"`
	DB                 string   `yaml:"db"`
	History            string   `yaml:"history_dir"`
	ServerURL          string   `yaml:"server_url"`
	AgentName          string   `yaml:"agent_name"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"`
}

const (
	defaultDB         = "./snapshot.db"
	defaultHistoryDir = "./history"
)

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

	dbRaw := raw.DB
	if dbRaw == "" {
		dbRaw = defaultDB
	}
	resolvedDB, err := resolvePath(dbRaw)
	if err != nil {
		return nil, fmt.Errorf("config %q: db %q: %w", path, dbRaw, err)
	}

	historyRaw := raw.History
	if historyRaw == "" {
		historyRaw = defaultHistoryDir
	}
	resolvedHistory, err := resolvePath(historyRaw)
	if err != nil {
		return nil, fmt.Errorf("config %q: history_dir %q: %w", path, historyRaw, err)
	}

	if raw.ServerURL != "" && raw.AgentName == "" {
		return nil, fmt.Errorf("config %q: 'agent_name' is required when 'server_url' is set", path)
	}

	cfg := &Config{
		Path:               resolvedRoot,
		Verbose:            raw.Verbose,
		DBPath:             resolvedDB,
		HistoryDir:         resolvedHistory,
		ServerURL:          raw.ServerURL,
		AgentName:          raw.AgentName,
		InsecureSkipVerify: raw.InsecureSkipVerify,
	}
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
