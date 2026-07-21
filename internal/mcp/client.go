package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIClient is a thin HTTP client for the hiveshare API,
// used by the MCP server to proxy tool calls.
type APIClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func NewAPIClient(baseURL, apiKey string) *APIClient {
	return &APIClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    http.DefaultClient,
	}
}

func (c *APIClient) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("api %d: %s", resp.StatusCode, raw)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (c *APIClient) ListHeadspaces(ctx context.Context) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	err := c.do(ctx, http.MethodGet, "/api/v1/hiveshares", nil, &result)
	return result, err
}

func (c *APIClient) SearchMemory(ctx context.Context, hiveshareID, query, sourceType string, limit int) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"query":       query,
		"source_type": sourceType,
		"limit":       limit,
	}
	var result map[string]interface{}
	path := fmt.Sprintf("/api/v1/hiveshares/%s/memory/search", hiveshareID)
	err := c.do(ctx, http.MethodPost, path, payload, &result)
	return result, err
}

func (c *APIClient) AddMemory(ctx context.Context, hiveshareID string, entry map[string]interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	path := fmt.Sprintf("/api/v1/hiveshares/%s/memory", hiveshareID)
	err := c.do(ctx, http.MethodPost, path, entry, &result)
	return result, err
}

func (c *APIClient) GetContext(ctx context.Context, hiveshareID, sourceRef string) (interface{}, error) {
	var result interface{}
	path := fmt.Sprintf("/api/v1/hiveshares/%s/memory?source_ref=%s&limit=100", hiveshareID, sourceRef)
	err := c.do(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (c *APIClient) GetMetrics(ctx context.Context, hiveshareID string) (interface{}, error) {
	var result interface{}
	path := fmt.Sprintf("/api/v1/hiveshares/%s/metrics", hiveshareID)
	err := c.do(ctx, http.MethodGet, path, nil, &result)
	return result, err
}
