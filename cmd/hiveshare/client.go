package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client is the HTTP client used by all hshare CLI commands to talk to the
// hiveshare server. It attaches the API key as a Bearer token on every request.
type Client struct {
	BaseURL string
	APIKey  string
}

// newClient builds a Client from the active CLI config. Returns an error if
// no API key is configured.
func newClient() (*Client, error) {
	cfg := loadConfig()
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("not logged in — run: hshare auth register --email you@example.com --name 'You'")
	}
	return &Client{BaseURL: cfg.ServerURL, APIKey: cfg.APIKey}, nil
}

func (c *Client) do(method, path string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection error: %w (is the server running?)", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e map[string]string
		json.Unmarshal(raw, &e)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, e["error"])
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (c *Client) get(path string, out interface{}) error {
	return c.do(http.MethodGet, path, nil, out)
}

func (c *Client) post(path string, body, out interface{}) error {
	return c.do(http.MethodPost, path, body, out)
}

func (c *Client) put(path string, body, out interface{}) error {
	return c.do(http.MethodPut, path, body, out)
}

func (c *Client) delete(path string) error {
	return c.do(http.MethodDelete, path, nil, nil)
}
