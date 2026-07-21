package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	mcpsrv "github.com/sagpaul/hiveshare/internal/mcp"
)

type Config struct {
	ServerURL        string `json:"server_url"`
	APIKey           string `json:"api_key"`
	DefaultHiveshare string `json:"default_hiveshare"`
}

func loadConfig() Config {
	// CLI flags / env override
	cfg := Config{
		ServerURL:        getenv("HIVESHARE_SERVER_URL", "http://localhost:8080"),
		APIKey:           os.Getenv("HIVESHARE_API_KEY"),
		DefaultHiveshare: os.Getenv("HIVESHARE_DEFAULT_HEADSPACE"),
	}

	// fall back to config file
	if cfg.APIKey == "" {
		cfgPath := filepath.Join(configDir(), "config.json")
		data, err := os.ReadFile(cfgPath)
		if err == nil {
			var fileCfg Config
			if json.Unmarshal(data, &fileCfg) == nil {
				if cfg.APIKey == "" {
					cfg.APIKey = fileCfg.APIKey
				}
				if cfg.ServerURL == "http://localhost:8080" && fileCfg.ServerURL != "" {
					cfg.ServerURL = fileCfg.ServerURL
				}
				if cfg.DefaultHiveshare == "" {
					cfg.DefaultHiveshare = fileCfg.DefaultHiveshare
				}
			}
		}
	}
	return cfg
}

func configDir() string {
	if d := os.Getenv("HIVESHARE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "hiveshare")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := loadConfig()
	if cfg.APIKey == "" {
		log.Fatal("HIVESHARE_API_KEY not set and no config file found at ~/.config/hiveshare/config.json")
	}

	client := mcpsrv.NewAPIClient(cfg.ServerURL, cfg.APIKey)
	srv := mcpsrv.NewServer(client, cfg.DefaultHiveshare, os.Stdin, os.Stdout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
