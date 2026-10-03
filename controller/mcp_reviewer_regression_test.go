package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// TestMCPProxyLatestRoutesExplicitLegacyVersionHeaders verifies versioned legacy clients bypass modern metadata validation.
//
// Parameters:
//   - t: The test owns the in-memory request and compatibility assertions.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestRoutesExplicitLegacyVersionHeaders(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"legacy-header","method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set(mcp.ProtocolVersionHeader, mcp.LegacyProtocolVersionFallback)
	context, response := newMCPCallContext(t, 1, "legacy-header")
	context.Request = request
	MCPProxyLatest(context)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"protocolVersion":"2025-06-18"`)
}

// TestMCPProxyLatestAllowsLargeLegacyBody verifies legacy compatibility is preserved above the modern four-megabyte limit.
//
// Parameters:
//   - t: The test owns the large initialization request and compatibility assertion.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestAllowsLargeLegacyBody(t *testing.T) {
	padding := strings.Repeat("x", int(modernMCPMaxRequestBytes)+1024)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "large-legacy",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcp.LegacyProtocolVersionFallback,
			"padding":         padding,
		},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	context, response := newMCPCallContext(t, 1, "large-legacy")
	context.Request = request
	MCPProxyLatest(context)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"protocolVersion":"2025-06-18"`)
}

// TestReadBoundedMCPRequestBodyRejectsOverflow verifies unversioned legacy fallback has a hard allocation boundary.
//
// Parameters:
//   - t: The test owns the bounded reader and overflow assertion.
//
// Return values: none; failures are reported through t.
func TestReadBoundedMCPRequestBodyRejectsOverflow(t *testing.T) {
	_, err := readBoundedMCPRequestBody(strings.NewReader("12345"), 4)
	require.Error(t, err)
	var tooLarge *mcpRequestTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	require.Equal(t, int64(4), tooLarge.Limit)
}

// TestMCPProxyLatestAcceptsLegacyInitializeWithoutParams verifies omitted optional initialize params retain compatibility.
//
// Parameters:
//   - t: The test owns the in-memory legacy handshake and response assertions.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestAcceptsLegacyInitializeWithoutParams(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"legacy-no-params","method":"initialize"}`)
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	context, response := newMCPCallContext(t, 1, "legacy-no-params")
	context.Request = request
	MCPProxyLatest(context)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"protocolVersion":"2025-11-25"`)
}

// TestMCPProxyLatestPreservesInvalidCursorError verifies method validation errors are not rewritten as internal failures.
//
// Parameters:
//   - t: The test owns the SQLite fixture and modern tools/list request.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestPreservesInvalidCursorError(t *testing.T) {
	cleanup, fixture := setupMCPProxyTest(t)
	defer cleanup()

	body := modernMCPRequestBody(t, "bad-cursor", "tools/list", map[string]any{"cursor": "%%%"})
	response := invokeModernMCPProxy(t, fixture, "bad-cursor", body, func(request *http.Request) {
		request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
		request.Header.Set(mcp.MethodHeader, "tools/list")
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, mcpErrInvalidParams, decodeMCPErrorCode(t, response))
}

// TestMCPProxyLatestPreservesToolHeaderMismatch verifies schema-driven validation errors keep the MCP header code.
//
// Parameters:
//   - t: The test owns the SQLite fixture and modern tools/call request.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestPreservesToolHeaderMismatch(t *testing.T) {
	cleanup, fixture := setupMCPProxyTest(t)
	defer cleanup()

	schema := `{"type":"object","properties":{"tenant":{"type":"string","x-mcp-header":"Tenant"}},"required":["tenant"]}`
	require.NoError(t, model.DB.Model(&model.MCPTool{}).Where("id = ?", fixture.tool.Id).Updates(map[string]any{
		"input_schema":    schema,
		"descriptor_json": "",
	}).Error)

	body := modernMCPRequestBody(t, "header-mismatch", "tools/call", map[string]any{
		"name":      "fake-mcp.echo",
		"arguments": map[string]any{"tenant": "acme"},
	})
	response := invokeModernMCPProxy(t, fixture, "header-mismatch", body, func(request *http.Request) {
		request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
		request.Header.Set(mcp.MethodHeader, "tools/call")
		request.Header.Set(mcp.NameHeader, "fake-mcp.echo")
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, mcp.ErrorCodeHeaderMismatch, decodeMCPErrorCode(t, response))
	require.Zero(t, fixture.upstreamHits)
}

// invokeModernMCPProxy invokes the authenticated modern endpoint with caller-provided headers.
//
// Parameters:
//   - t: The test receives fixture and decoding failures.
//   - fixture: The fixture supplies the authenticated user and database state.
//   - requestID: The request identifier is attached to tracing context.
//   - body: The complete JSON-RPC request body is submitted.
//   - configure: The optional callback adds method-specific request headers.
//
// Return values:
//   - *httptest.ResponseRecorder: The captured HTTP response is returned.
func invokeModernMCPProxy(t *testing.T, fixture *mcpFixture, requestID string, body []byte, configure func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	context, response := newMCPCallContext(t, fixture.user.Id, requestID)
	context.Set(ctxkey.UserObj, fixture.user)
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if configure != nil {
		configure(request)
	}
	context.Request = request
	MCPProxyLatest(context)
	return response
}

// decodeMCPErrorCode extracts one JSON-RPC error code from an HTTP response.
//
// Parameters:
//   - t: The test receives JSON decoding failures.
//   - response: The recorded response contains a JSON-RPC error envelope.
//
// Return values:
//   - int: The decoded error code is returned.
func decodeMCPErrorCode(t *testing.T, response *httptest.ResponseRecorder) int {
	t.Helper()
	var envelope struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	return envelope.Error.Code
}
