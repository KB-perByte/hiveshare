package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client is the HTTP client used by all hiveshare CLI commands to talk to the
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
		return nil, fmt.Errorf("not logged in — run: hiveshare auth register --email you@example.com --name 'You'")
	}
	return &Client{BaseURL: cfg.ServerURL, APIKey: cfg.APIKey}, nil
}

func (c *Client) do(method, path string, body interface{}, out interface{}) error {
	_, err := c.doRaw(method, path, body, out)
	return err
}

// doRaw executes the request and returns the raw response body alongside any
// error. When out is non-nil the body is also decoded into it.
func (c *Client) doRaw(method, path string, body interface{}, out interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w (is the server running?)", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e map[string]string
		json.Unmarshal(raw, &e)
		return raw, fmt.Errorf("server error %d: %s", resp.StatusCode, e["error"])
	}
	if out != nil {
		return raw, json.Unmarshal(raw, out)
	}
	return raw, nil
}

func (c *Client) get(path string, out interface{}) error {
	return c.do(http.MethodGet, path, nil, out)
}

// getRaw returns the raw JSON body for --json passthrough.
func (c *Client) getRaw(path string) ([]byte, error) {
	return c.doRaw(http.MethodGet, path, nil, nil)
}

// postRaw returns the raw JSON body for --json passthrough.
func (c *Client) postRaw(path string, body interface{}) ([]byte, error) {
	return c.doRaw(http.MethodPost, path, body, nil)
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
