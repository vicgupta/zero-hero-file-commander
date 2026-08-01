package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// cfg is the persisted user configuration (options only — the panels always
// open in the current directory, never a remembered one; see main.go).
type cfg struct {
	Brief  bool `json:"brief"`
	Hidden bool `json:"hidden"`
	// ViKeys enables h/j/k/l/n/u navigation and fast filename search while
	// the command line is dormant.
	ViKeys bool `json:"vi_keys"`
	// Theme selects a palette from themeNames ("norton", "nightowl", "opencode").
	Theme string `json:"theme"`
}

func configPath() string {
	if dir := os.Getenv("NC_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	return filepath.Join(dir, "zhfc", "config.json")
}

func loadConfig() cfg {
	c := cfg{ViKeys: true, Theme: defaultTheme}
	b, err := os.ReadFile(configPath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c)
	if _, ok := themes[c.Theme]; !ok {
		c.Theme = defaultTheme
	}
	return c
}

func (c cfg) save() {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
}
