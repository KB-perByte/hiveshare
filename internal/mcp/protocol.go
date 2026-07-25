// Package mcp implements the MCP (Model Context Protocol) stdio server and the
// HTTP API client proxy that backs its tool calls.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// JSON-RPC 2.0 types used by the MCP server and client.

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func Errorf(code int, format string, args ...interface{}) *RPCError {
	return &RPCError{Code: code, Message: fmt.Sprintf(format, args...)}
}

type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Tool definition for MCP initialize response
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func WriteResponse(w io.Writer, resp Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

func WriteNotification(w io.Writer, n Notification) error {
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

func ReadRequest(r *bufio.Reader) (*Request, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
