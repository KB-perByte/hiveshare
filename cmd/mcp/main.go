// Command mcp starts the HiveShare MCP stdio server, which allows AI assistants
// (Claude, Cursor, etc.) to save and retrieve hive context via MCP tool calls.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	mcpsrv "github.com/KB-perByte/hiveshare/internal/mcp"
)

// Config holds the MCP server runtime configuration, loaded from environment
// variables and the shared CLI config file.
type Config struct {
	ServerURL        string `json:"server_url"`
	APIKey           string `json:"api_key"`
	DefaultHiveshare string `json:"default_hiveshare"`
}

func loadConfig() Config {
	// CLI flags / env override
	cfg := Config{
		ServerURL: getenv("HIVESHARE_SERVER_URL", "http://localhost:8080"),
		APIKey:    os.Getenv("HIVESHARE_API_KEY"),
		// Use HIVESHARE_DEFAULT_HIVESHARE; HIVESHARE_DEFAULT_HEADSPACE kept for backwards compat.
		DefaultHiveshare: firstNonEmpty(
			os.Getenv("HIVESHARE_DEFAULT_HIVESHARE"),
			os.Getenv("HIVESHARE_DEFAULT_HEADSPACE"),
		),
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func main() {
	cfg := loadConfig()
	if cfg.APIKey == "" {
		// Allow startup without a key so accept_invite can run during first-time
		// onboarding. Every other tool will return an auth error until the key is
		// configured and the process is restarted.
		log.Println("hiveshare-mcp: no API key configured — only accept_invite is available. " +
			"Run accept_invite to get your key, then set HIVESHARE_API_KEY and restart.")
	}

	client := mcpsrv.NewAPIClient(cfg.ServerURL, cfg.APIKey)
	srv := mcpsrv.NewServer(client, cfg.DefaultHiveshare, os.Stdin, os.Stdout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
