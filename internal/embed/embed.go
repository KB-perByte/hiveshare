package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Embedder is the pluggable embedding interface.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
}

// NoOpEmbedder skips embedding — search falls back to full-text.
type NoOpEmbedder struct{}

func (n *NoOpEmbedder) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (n *NoOpEmbedder) Dimensions() int                                       { return 0 }

// New returns the configured embedder based on EMBED_PROVIDER env var.
// Defaults to NoOpEmbedder when not configured.
func New() Embedder {
	switch os.Getenv("EMBED_PROVIDER") {
	case "openai":
		return &OpenAIEmbedder{
			APIKey: os.Getenv("OPENAI_API_KEY"),
			Model:  getenv("OPENAI_EMBED_MODEL", "text-embedding-3-small"),
		}
	case "ollama":
		return &OllamaEmbedder{
			BaseURL: getenv("OLLAMA_BASE_URL", "http://localhost:11434"),
			Model:   getenv("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
		}
	default:
		return &NoOpEmbedder{}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// OpenAIEmbedder calls the OpenAI embeddings API.
type OpenAIEmbedder struct {
	APIKey string
	Model  string
}

func (e *OpenAIEmbedder) Dimensions() int { return 1536 }

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": e.Model,
		"input": text,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embed %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openai embed: empty response")
	}
	return result.Data[0].Embedding, nil
}

// OllamaEmbedder calls the local Ollama embeddings API.
type OllamaEmbedder struct {
	BaseURL string
	Model   string
}

func (e *OllamaEmbedder) Dimensions() int { return 768 }

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":  e.Model,
		"prompt": text,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		e.BaseURL+"/api/embeddings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}
