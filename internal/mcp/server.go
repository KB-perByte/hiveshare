package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
)

const serverVersion = "0.1.0"

// Server is the MCP stdio server. It reads JSON-RPC requests from stdin and
// writes responses to stdout, proxying tool calls to the hiveshare HTTP API via
// an embedded APIClient.
type Server struct {
	client         *APIClient
	defaultHS      string // default hiveshare ID
	reader         *bufio.Reader
	writer         io.Writer
	initialized    bool
}

// NewServer creates a Server that uses client to proxy tool calls and defaults
// to defaultHiveshare when the caller omits a hiveshare_id argument.
func NewServer(client *APIClient, defaultHiveshare string, reader io.Reader, writer io.Writer) *Server {
	return &Server{
		client:    client,
		defaultHS: defaultHiveshare,
		reader:    bufio.NewReader(reader),
		writer:    writer,
	}
}

// Run reads JSON-RPC requests until EOF or ctx cancellation, dispatching each
// synchronously and writing the response before reading the next.
func (s *Server) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		req, err := ReadRequest(s.reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			slog.Error("mcp: read error", "err", err)
			continue
		}

		resp := s.handle(ctx, req)
		if err := WriteResponse(s.writer, resp); err != nil {
			slog.Error("mcp: write error", "err", err)
			return err
		}
	}
}

func (s *Server) handle(ctx context.Context, req *Request) Response {
	base := Response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(base, req)
	case "tools/list":
		return s.handleToolsList(base)
	case "tools/call":
		return s.handleToolCall(ctx, base, req)
	case "ping":
		base.Result = map[string]string{"status": "ok"}
		return base
	default:
		base.Error = Errorf(-32601, "method not found: %s", req.Method)
		return base
	}
}

func (s *Server) handleInitialize(base Response, req *Request) Response {
	s.initialized = true
	base.Result = map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    "hiveshare",
			"version": serverVersion,
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]bool{"listChanged": false},
		},
	}
	return base
}

func (s *Server) handleToolsList(base Response) Response {
	base.Result = map[string]interface{}{
		"tools": tools(),
	}
	return base
}

func (s *Server) handleToolCall(ctx context.Context, base Response, req *Request) Response {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		base.Error = Errorf(-32600, "invalid params")
		return base
	}

	result, err := s.callTool(ctx, params.Name, params.Arguments)
	if err != nil {
		base.Error = Errorf(-32000, err.Error())
		return base
	}

	// MCP tools/call result wraps content
	text, _ := json.MarshalIndent(result, "", "  ")
	base.Result = map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": string(text)},
		},
	}
	return base
}

func (s *Server) callTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	hsID := stringArg(args, "hiveshare_id")
	if hsID == "" {
		hsID = s.defaultHS
	}

	if hsID == "" && name != "create_hiveshare" && name != "list_hiveshares" {
		return nil, Errorf(-32602, "no hiveshare selected — create one first with create_hiveshare, or pass hiveshare_id")
	}

	switch name {
	case "create_hiveshare":
		name := stringArg(args, "name")
		if name == "" {
			return nil, Errorf(-32602, "name is required")
		}
		return s.client.CreateHiveshare(ctx, name, stringArg(args, "description"))

	case "list_hiveshares":
		return s.client.ListHiveshares(ctx)

	case "search_hives":
		query := stringArg(args, "query")
		if query == "" {
			return nil, Errorf(-32602, "query is required")
		}
		sourceType := stringArg(args, "source_type")
		limit := intArg(args, "limit", 10)
		return s.client.SearchHives(ctx, hsID, query, sourceType, limit)

	case "add_hive":
		content := stringArg(args, "content")
		sourceType := stringArg(args, "source_type")
		sourceRef := stringArg(args, "source_ref")
		if content == "" || sourceType == "" || sourceRef == "" {
			return nil, Errorf(-32602, "content, source_type, and source_ref are required")
		}
		entry := map[string]interface{}{
			"content":     content,
			"source_type": sourceType,
			"source_ref":  sourceRef,
			"source_url":  stringArg(args, "source_url"),
			"summary":     stringArg(args, "summary"),
			"tool":        stringArgDefault(args, "tool", "claude"),
			"tags":        sliceArg(args, "tags"),
		}
		return s.client.AddHive(ctx, hsID, entry)

	case "get_context":
		sourceRef := stringArg(args, "source_ref")
		if sourceRef == "" {
			return nil, Errorf(-32602, "source_ref is required")
		}
		return s.client.GetContext(ctx, hsID, sourceRef)

	case "get_metrics":
		return s.client.GetMetrics(ctx, hsID)

	case "list_hives":
		sourceType := stringArg(args, "source_type")
		limit := intArg(args, "limit", 20)
		offset := intArg(args, "offset", 0)
		return s.client.ListHives(ctx, hsID, sourceType, limit, offset)

	case "update_hive":
		entryID := stringArg(args, "entry_id")
		if entryID == "" {
			return nil, Errorf(-32602, "entry_id is required")
		}
		content := stringArg(args, "content")
		if content == "" {
			return nil, Errorf(-32602, "content is required")
		}
		payload := map[string]interface{}{
			"content": content,
			"summary": stringArg(args, "summary"),
			"tags":    sliceArg(args, "tags"),
		}
		return s.client.UpdateHive(ctx, hsID, entryID, payload)

	case "delete_hive":
		entryID := stringArg(args, "entry_id")
		if entryID == "" {
			return nil, Errorf(-32602, "entry_id is required")
		}
		if err := s.client.DeleteHive(ctx, hsID, entryID); err != nil {
			return nil, err
		}
		return map[string]string{"status": "deleted", "entry_id": entryID}, nil

	case "batch_add":
		rawEntries, ok := args["entries"].([]interface{})
		if !ok || len(rawEntries) == 0 {
			return nil, Errorf(-32602, "entries must be a non-empty array")
		}
		var entries []map[string]interface{}
		for _, raw := range rawEntries {
			if m, ok := raw.(map[string]interface{}); ok {
				if m["tool"] == nil {
					m["tool"] = "claude"
				}
				entries = append(entries, m)
			}
		}
		return s.client.BatchAdd(ctx, hsID, entries)

	default:
		return nil, Errorf(-32601, "unknown tool: %s", name)
	}
}

// tools returns the MCP tool definitions.
func tools() []Tool {
	return []Tool{
		{
			Name:        "create_hiveshare",
			Description: "Create a new hiveshare for your team. Do this first if list_hiveshares returns empty.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name":        {Type: "string", Description: "Name for the hiveshare"},
					"description": {Type: "string", Description: "Optional description"},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "search_hives",
			Description: "Search hives in a hiveshare using semantic or full-text search. Use this before asking the user about context — the hive may already have what you need.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query":        {Type: "string", Description: "Search query"},
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
					"source_type":  {Type: "string", Description: "Filter by source type: jira, github_issue, github_pr, file, url, manual"},
					"limit":        {Type: "integer", Description: "Max results (default 10)"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "add_hive",
			Description: "Save crunched context to the hiveshare so teammates can reuse it. Call this after you process a Jira ticket, GitHub issue, PR, or any other artifact.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"content":      {Type: "string", Description: "The full crunched/processed context"},
					"source_type":  {Type: "string", Description: "Type: jira, github_issue, github_pr, file, url, manual"},
					"source_ref":   {Type: "string", Description: "Reference ID, e.g. PROJ-123 or owner/repo#42"},
					"source_url":   {Type: "string", Description: "URL to the source (optional)"},
					"summary":      {Type: "string", Description: "Short one-line summary (optional)"},
					"tool":         {Type: "string", Description: "Tool used: claude, cursor, manual"},
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
					"tags":         {Type: "string", Description: "Comma-separated tags (optional)"},
				},
				Required: []string{"content", "source_type", "source_ref"},
			},
		},
		{
			Name:        "list_hiveshares",
			Description: "List all hiveshares you have access to, with member counts and your role.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "get_context",
			Description: "Get all hives for a specific source reference (e.g. all notes on PROJ-123). Use to load full team context for a ticket or issue.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"source_ref":   {Type: "string", Description: "The source reference, e.g. PROJ-123 or owner/repo#42"},
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
				},
				Required: []string{"source_ref"},
			},
		},
		{
			Name:        "get_metrics",
			Description: "Get collaboration and memory metrics for a hiveshare, including reuse rate, top contributors, and activity stats.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
				},
			},
		},
		{
			Name:        "list_hives",
			Description: "List hives in a hiveshare with optional filtering by source type. Use before searching to browse what's available.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
					"source_type":  {Type: "string", Description: "Filter by type: jira, github_issue, github_pr, file, url, manual"},
					"limit":        {Type: "integer", Description: "Max results (default 20)"},
					"offset":       {Type: "integer", Description: "Pagination offset (default 0)"},
				},
			},
		},
		{
			Name:        "update_hive",
			Description: "Update an existing hive's content, summary, or tags. Use to refine or correct a hive you previously added.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entry_id":     {Type: "string", Description: "UUID of the hive to update"},
					"content":      {Type: "string", Description: "New full content (required)"},
					"summary":      {Type: "string", Description: "Updated one-line summary"},
					"tags":         {Type: "string", Description: "Comma-separated tags"},
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
				},
				Required: []string{"entry_id", "content"},
			},
		},
		{
			Name:        "delete_hive",
			Description: "Delete a hive entry. Use to remove stale or incorrect entries.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entry_id":     {Type: "string", Description: "UUID of the hive to delete"},
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
				},
				Required: []string{"entry_id"},
			},
		},
		{
			Name:        "batch_add",
			Description: "Add multiple hives in one call. More efficient than calling add_hive repeatedly when you have several artifacts to store.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entries":      {Type: "array", Description: "Array of hive objects, each with content, source_type, source_ref (and optionally summary, tags, source_url)"},
					"hiveshare_id": {Type: "string", Description: "Hiveshare UUID (uses default if omitted)"},
				},
				Required: []string{"entries"},
			},
		},
	}
}

func stringArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

func stringArgDefault(args map[string]interface{}, key, def string) string {
	if v := stringArg(args, key); v != "" {
		return v
	}
	return def
}

func intArg(args map[string]interface{}, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

func sliceArg(args map[string]interface{}, key string) []string {
	switch v := args[key].(type) {
	case []interface{}:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}
