package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/model"
)

// Client defines MCP operations required by the aggregator.
type Client interface {
	ListTools(ctx context.Context) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error)
}

// StreamableHTTPClient implements MCP client calls over HTTP JSON-RPC.
type StreamableHTTPClient struct {
	BaseURL string
	Headers map[string]string
	Timeout time.Duration
}

// NewStreamableHTTPClient constructs a StreamableHTTPClient from MCP server metadata.
func NewStreamableHTTPClient(server *model.MCPServer, headers map[string]string, timeout time.Duration) *StreamableHTTPClient {
	merged := make(map[string]string)
	for k, v := range server.Headers {
		merged[k] = v
	}
	for k, v := range headers {
		merged[k] = v
	}

	switch strings.ToLower(server.AuthType) {
	case model.MCPAuthTypeBearer:
		if server.APIKey != "" {
			merged["Authorization"] = "Bearer " + server.APIKey
		}
	case model.MCPAuthTypeAPIKey:
		if server.APIKey != "" {
			merged["X-API-Key"] = server.APIKey
		}
	}

	return &StreamableHTTPClient{
		BaseURL: strings.TrimSpace(server.BaseURL),
		Headers: merged,
		Timeout: timeout,
	}
}

// ListTools calls the MCP tools/list method.
func (c *StreamableHTTPClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	var result struct {
		Tools []ToolDescriptor `json:"tools"`
	}
	if err := c.doRPC(ctx, "tools/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a MCP tool by name.
func (c *StreamableHTTPClient) CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	var result CallToolResult
	if err := c.doRPC(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// doRPC performs a JSON-RPC call against the MCP server.
func (c *StreamableHTTPClient) doRPC(ctx context.Context, method string, params any, out any) error {
	if c == nil {
		return errors.New("mcp client is nil")
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      random.GetUUID(),
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal mcp request")
	}

	client := &http.Client{Timeout: c.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "create mcp request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "send mcp request")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("mcp request failed with status %d", resp.StatusCode)
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  map[string]any  `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return errors.Wrap(err, "decode mcp response")
	}
	if envelope.Error != nil {
		return errors.Errorf("mcp error: %v", envelope.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return errors.Wrap(err, "unmarshal mcp result")
	}
	return nil
}
