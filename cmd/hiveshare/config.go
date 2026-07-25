package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CLIConfig holds the persisted CLI configuration. Fields are read from the
// JSON config file and can be overridden by environment variables.
type CLIConfig struct {
	ServerURL        string `json:"server_url"`
	APIKey           string `json:"api_key"`
	DefaultHiveshare string `json:"default_hiveshare"`
	DefaultHSName    string `json:"default_hiveshare_name"`
}

// configPath returns the path to the JSON config file, respecting
// HIVESHARE_CONFIG_DIR if set.
func configPath() string {
	if d := os.Getenv("HIVESHARE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "hiveshare", "config.json")
}

// loadConfig returns the effective CLI config, merging environment variables
// (highest priority) over the persisted JSON file.
func loadConfig() CLIConfig {
	cfg := CLIConfig{
		ServerURL: getenvOrDefault("HIVESHARE_SERVER_URL", "http://localhost:8080"),
		APIKey:    os.Getenv("HIVESHARE_API_KEY"),
	}
	data, err := os.ReadFile(configPath())
	if err == nil {
		var fileCfg CLIConfig
		if json.Unmarshal(data, &fileCfg) == nil {
			if cfg.APIKey == "" {
				cfg.APIKey = fileCfg.APIKey
			}
			if cfg.ServerURL == "http://localhost:8080" && fileCfg.ServerURL != "" {
				cfg.ServerURL = fileCfg.ServerURL
			}
			if cfg.DefaultHiveshare == "" {
				cfg.DefaultHiveshare = fileCfg.DefaultHiveshare
				cfg.DefaultHSName = fileCfg.DefaultHSName
			}
		}
	}
	return cfg
}

// saveConfig atomically writes cfg to the JSON config file, creating the
// directory if it does not exist. The file is mode 0600 (owner-read only).
func saveConfig(cfg CLIConfig) error {
	p := configPath()
	os.MkdirAll(filepath.Dir(p), 0700)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

func getenvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
