package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfig_Jitter(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "gofim.yml")
	content := `path: /tmp
jitter: 45s
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}

	if cfg.Jitter != 45*time.Second {
		t.Errorf("cfg.Jitter = %v, want 45s", cfg.Jitter)
	}
}

func TestConfig_JitterSeconds(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "gofim.yml")
	content := `path: /tmp
jitter: "15"
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}

	if cfg.Jitter != 15*time.Second {
		t.Errorf("cfg.Jitter = %v, want 15s", cfg.Jitter)
	}
}
