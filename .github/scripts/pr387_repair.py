from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def update(path: str, transform) -> None:
    target = ROOT / path
    source = target.read_text()
    updated = transform(source)
    if updated != source:
        target.write_text(updated)


def replace_function(source: str, signature: str, next_comment: str, replacement: str) -> str:
    start = source.find(signature)
    end = source.find(next_comment, start)
    if start < 0 or end < 0:
        raise RuntimeError(f"cannot replace function {signature}")
    return source[:start] + replacement.rstrip() + "\n\n" + source[end:]


def fix_call_latest(source: str) -> str:
    source = source.replace('\n\t"github.com/Laisky/one-api/model"', '')
    source = source.replace('startedAt := time.Now()', 'startedAt := time.Now().UTC()')
    source = source.replace(
        'mcp.NewStreamableHTTPClientWithLogger(server, buildMCPHeaders(server, c),',
        'mcp.NewStreamableHTTPClientWithLogger(server, nil,',
    )
    source = re.sub(
        r'return nil, errors\.New\(("(?:tool name is required|mcp server not loaded)")\)',
        r'return nil, errors.WithStack(errors.New(\1))',
        source,
    )
    return source


update('controller/mcp_call_latest.go', fix_call_latest)

for name in ('controller/mcp_proxy_latest_test.go', 'controller/mcp_proxy_test.go'):
    update(name, lambda source: source.replace('mcpProtocolVersion', 'mcp.LegacyProtocolVersionFallback'))


def fix_legacy_proxy(source: str) -> str:
    original = '''\t\tvar params mcpInitializeParams
\t\tif err := json.Unmarshal(request.Params, &params); err != nil {
\t\t\trespondMCPError(c, request.ID, mcpErrInvalidParams, errors.Wrap(err, "decode mcp initialize params"))
\t\t\treturn
\t\t}
'''
    replacement = '''\t\tvar params mcpInitializeParams
\t\tif len(request.Params) != 0 {
\t\t\tif err := json.Unmarshal(request.Params, &params); err != nil {
\t\t\t\trespondMCPError(c, request.ID, mcpErrInvalidParams, errors.Wrap(err, "decode mcp initialize params"))
\t\t\t\treturn
\t\t\t}
\t\t}
'''
    source = source.replace(original, replacement)
    source = re.sub(
        r'return nil, errors\.New\(("(?:tool name is required|no eligible MCP tool found|mcp server not loaded)")\)',
        r'return nil, errors.WithStack(errors.New(\1))',
        source,
    )
    return source


update('controller/mcp_proxy.go', fix_legacy_proxy)
update('relay/mcp/client_latest.go', lambda source: source.replace('"arguments": arguments,', '"arguments": argumentMap,'))
update('relay/mcp/protocol.go', lambda source: source.replace('make(map[string]any, len(params)+1)', 'make(map[string]any, len(params))'))
update(
    'docs/manuals/mcp_protocol_2026_07_28.md',
    lambda source: source.replace('params._meta["io.modelcontextprotocol/serverInfo"]', 'result._meta["io.modelcontextprotocol/serverInfo"]'),
)


def fix_modern_proxy(source: str) -> str:
    if 'stderrors "errors"' not in source:
        source = source.replace('import (\n\t"bytes"', 'import (\n\t"bytes"\n\tstderrors "errors"', 1)
    source = re.sub(
        r'const modernMCPMaxRequestBytes(?: int64)? = 4 << 20',
        'const (\n\tmodernMCPMaxRequestBytes int64 = 4 << 20\n\tlegacyMCPMaxRequestBytes int64 = 32 << 20\n)',
        source,
        count=1,
    )
    proxy = r'''func MCPProxyLatest(c *gin.Context) {
	if err := validateModernMCPOrigin(c.Request); err != nil {
		respondMCPModernError(c, nil, http.StatusForbidden, mcpErrInvalidRequest, err, nil)
		return
	}

	versionValues := c.Request.Header.Values(mcp.ProtocolVersionHeader)
	if len(versionValues) > 1 {
		respondMCPModernError(c, nil, http.StatusBadRequest, mcp.ErrorCodeHeaderMismatch, errors.Errorf("%s must occur at most once", mcp.ProtocolVersionHeader), nil)
		return
	}
	modernTransport := len(versionValues) == 1 && strings.TrimSpace(versionValues[0]) == mcp.ProtocolVersion

	switch c.Request.Method {
	case http.MethodGet, http.MethodDelete:
		if modernTransport {
			c.Header("Allow", http.MethodPost)
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}
		MCPProxy(c)
		return
	case http.MethodPost:
		// Continue below.
	default:
		c.Header("Allow", http.MethodPost)
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}

	originalBody := c.Request.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, legacyMCPMaxRequestBytes+1))
	if err != nil {
		respondMCPModernError(c, nil, http.StatusBadRequest, mcpErrParseError, errors.Wrap(err, "read mcp request"), nil)
		return
	}
	if int64(len(body)) > legacyMCPMaxRequestBytes {
		respondMCPModernError(c, nil, http.StatusRequestEntityTooLarge, mcpErrInvalidRequest, errors.Errorf("mcp request body exceeds %d bytes", legacyMCPMaxRequestBytes), nil)
		return
	}
	c.Request.Body = &replayReadCloser{Reader: bytes.NewReader(body), closer: originalBody}

	if len(versionValues) == 1 && mcp.IsLegacyProtocolVersion(versionValues[0]) {
		MCPProxy(c)
		return
	}

	var request mcpRPCRequest
	if err := json.Unmarshal(body, &request); err != nil {
		if len(versionValues) == 0 {
			MCPProxy(c)
			return
		}
		respondMCPModernError(c, nil, http.StatusBadRequest, mcpErrParseError, errors.Wrap(err, "decode modern mcp request"), nil)
		return
	}
	if !isModernMCPRequest(c, request) {
		MCPProxy(c)
		return
	}
	if int64(len(body)) > modernMCPMaxRequestBytes {
		respondMCPModernError(c, request.ID, http.StatusRequestEntityTooLarge, mcpErrInvalidRequest, errors.Errorf("modern mcp request body exceeds %d bytes", modernMCPMaxRequestBytes), nil)
		return
	}
	if err := validateModernMCPRequest(c, request); err != nil {
		respondModernValidationError(c, request.ID, err)
		return
	}
	handleModernMCPPost(c, request)
}'''
    source = replace_function(source, 'func MCPProxyLatest(c *gin.Context) {', '\n// isModernMCPRequest ', proxy)

    if 'func (e *modernMCPValidationError) Unwrap() error' not in source:
        marker = '''func (e *modernMCPValidationError) Error() string {
\tif e == nil || e.Err == nil {
\t\treturn "invalid modern mcp request"
\t}
\treturn e.Err.Error()
}
'''
        source = source.replace(marker, marker + '''
// Unwrap returns the underlying validation error for errors.Is and errors.As.
//
// Parameters: none.
//
// Return values:
//   - error: The underlying validation error is returned, or nil for an empty receiver.
func (e *modernMCPValidationError) Unwrap() error {
\tif e == nil {
\t\treturn nil
\t}
\treturn e.Err
}
''')

    validation = r'''func respondModernValidationError(c *gin.Context, id any, err error) {
	var validationErr *modernMCPValidationError
	if !stderrors.As(err, &validationErr) || validationErr == nil {
		respondMCPModernError(c, id, http.StatusBadRequest, mcpErrInvalidRequest, err, nil)
		return
	}
	respondMCPModernError(c, id, validationErr.Status, validationErr.Code, validationErr.Err, validationErr.Data)
}'''
    source = replace_function(source, 'func respondModernValidationError(c *gin.Context, id any, err error) {', '\n// handleModernMCPPost ', validation)

    dispatch = r'''func handleModernMCPPost(c *gin.Context, request mcpRPCRequest) {
	switch strings.TrimSpace(request.Method) {
	case "server/discover":
		respondMCPModernResult(c, request.ID, mcp.DiscoveryResult{
			ResultType:        mcp.ResultTypeComplete,
			SupportedVersions: mcp.SupportedProtocolVersions(),
			Capabilities: gin.H{
				"tools": gin.H{"listChanged": false},
			},
			Instructions: "Use tools/list to enumerate the authenticated aggregate catalog, then call tools/call with the exact qualified case-sensitive name returned by tools/list.",
			TTLMS:        3600000,
			CacheScope:   mcp.CacheScopePrivate,
			Meta:         mcp.ServerResponseMeta(mcpServerName, mcpServerVersion),
		})
	case "tools/list":
		result, err := listModernMCPToolsPage(gmw.Ctx(c), c, request.Params)
		if err != nil {
			respondModernDispatchError(c, request.ID, err)
			return
		}
		respondMCPModernResult(c, request.ID, result)
	case "tools/call":
		var params modernMCPCallParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			respondMCPModernError(c, request.ID, http.StatusBadRequest, mcpErrInvalidParams, errors.Wrap(err, "decode mcp call params"), nil)
			return
		}
		result, err := executeModernMCPTool(gmw.Ctx(c), c, params)
		if err != nil {
			respondModernDispatchError(c, request.ID, err)
			return
		}
		respondMCPModernResult(c, request.ID, result)
	default:
		if request.ID == nil {
			c.AbortWithStatus(http.StatusAccepted)
			return
		}
		respondMCPModernError(c, request.ID, http.StatusNotFound, mcpErrMethodNotFound, errors.Errorf("unsupported method %s", request.Method), nil)
	}
}

// respondModernDispatchError preserves protocol validation details and redacts unexpected internal failures.
//
// Parameters:
//   - c: The Gin request context receives the JSON-RPC response and supplies request-scoped logging.
//   - id: The JSON-RPC request identifier is reflected in the response.
//   - err: The wrapped method failure is classified as validation or internal.
//
// Return values: none; the complete JSON-RPC error response is written through c.
func respondModernDispatchError(c *gin.Context, id any, err error) {
	var validationErr *modernMCPValidationError
	if stderrors.As(err, &validationErr) && validationErr != nil {
		respondModernValidationError(c, id, validationErr)
		return
	}
	respondMCPModernError(c, id, http.StatusOK, mcpErrInternal, err, nil)
}'''
    source = replace_function(source, 'func handleModernMCPPost(c *gin.Context, request mcpRPCRequest) {', '\n// respondMCPModernResult ', dispatch)
    return source


update('controller/mcp_proxy_latest.go', fix_modern_proxy)


def fix_types(source: str) -> str:
    if 'func rawJSONIsNull(' not in source:
        if '\t"strings"\n' not in source:
            source = source.replace('import (\n', 'import (\n\t"strings"\n', 1)
        source += '''

// rawJSONIsNull reports whether an optional raw JSON field explicitly contains null.
//
// Parameters:
//   - raw: The encoded JSON field value is inspected without modification.
//
// Return values:
//   - bool: True is returned only for the JSON null literal with optional surrounding whitespace.
func rawJSONIsNull(raw json.RawMessage) bool {
\treturn strings.TrimSpace(string(raw)) == "null"
}
'''
    pattern = re.compile(r'func (decode\w+)\(([^)]*?)\) ([^{\n]+) \{')
    additions: list[tuple[int, str]] = []
    for match in pattern.finditer(source):
        parameters = match.group(2)
        returns = match.group(3)
        if 'json.RawMessage' not in parameters or not any(token in returns for token in ('map[', '[]', 'any')):
            continue
        raw_match = re.search(r'(\w+)\s+json\.RawMessage', parameters)
        if raw_match is None:
            continue
        raw_name = raw_match.group(1)
        if f'rawJSONIsNull({raw_name})' in source[match.end():match.end() + 200]:
            continue
        additions.append((match.end(), f'\n\tif rawJSONIsNull({raw_name}) {{\n\t\treturn nil, nil\n\t}}'))
    for offset, addition in reversed(additions):
        source = source[:offset] + addition + source[offset:]
    source = re.sub(
        r'if encoded := ([^;]+); len\(encoded\) != 0 \{',
        r'if encoded := \1; len(encoded) != 0 && !rawJSONIsNull(encoded) {',
        source,
    )
    return source


update('relay/mcp/types.go', fix_types)


def fix_client_tests(source: str) -> str:
    source = source.replace('_ = json.NewEncoder(w).Encode(', 'writeMCPTestJSON(t, w, ')
    source = source.replace('_, _ = fmt.Fprint(w,', 'writeMCPTestText(t, w,')
    if 'func writeMCPTestJSON(' not in source:
        source += '''

// writeMCPTestJSON encodes one mock HTTP response and fails the owning test at the write site.
//
// Parameters:
//   - t: The owning test receives any encoding failure.
//   - writer: The mock HTTP response writer receives the JSON payload.
//   - value: The JSON-compatible response value is encoded.
//
// Return values: none; failures are reported through t.
func writeMCPTestJSON(t *testing.T, writer http.ResponseWriter, value any) {
\tt.Helper()
\trequire.NoError(t, json.NewEncoder(writer).Encode(value))
}

// writeMCPTestText writes one mock HTTP response and fails the owning test at the write site.
//
// Parameters:
//   - t: The owning test receives any write failure.
//   - writer: The mock HTTP response writer receives the text payload.
//   - value: The response text is written exactly once.
//
// Return values: none; failures are reported through t.
func writeMCPTestText(t *testing.T, writer http.ResponseWriter, value string) {
\tt.Helper()
\t_, err := fmt.Fprint(writer, value)
\trequire.NoError(t, err)
}
'''
    return source


update('relay/mcp/client_latest_test.go', fix_client_tests)


def fix_server_transport(source: str) -> str:
    if '\t"net"\n' not in source:
        source = source.replace('import (\n', 'import (\n\t"net"\n', 1)
    needle = '''\tif parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
\t\treturn errkind.InvalidRequestErr(errors.New("mcp server base_url must use http or https"))
\t}
'''
    if 'credentialed mcp server base_url must use https' not in source:
        source = source.replace(needle, needle + '''\tif parsedURL.Scheme != "https" && s.hasSensitiveTransportConfiguration() && !isLoopbackMCPHost(parsedURL.Hostname()) {
\t\treturn errkind.InvalidRequestErr(errors.New("credentialed mcp server base_url must use https unless it targets loopback"))
\t}
''')
    if 'func (s *MCPServer) hasSensitiveTransportConfiguration() bool' not in source:
        source += '''

// hasSensitiveTransportConfiguration reports whether a server configuration sends credentials or secret-like headers.
//
// Parameters: none.
//
// Return values:
//   - bool: True is returned when the server carries an API key or a sensitive custom header.
func (s *MCPServer) hasSensitiveTransportConfiguration() bool {
\tif s == nil {
\t\treturn false
\t}
\tif strings.TrimSpace(s.APIKey) != "" {
\t\treturn true
\t}
\tfor key := range s.Headers {
\t\tlower := strings.ToLower(strings.TrimSpace(key))
\t\tfor _, token := range []string{"authorization", "api-key", "apikey", "token", "secret", "password", "cookie"} {
\t\t\tif strings.Contains(lower, token) {
\t\t\t\treturn true
\t\t\t}
\t\t}
\t}
\treturn false
}

// isLoopbackMCPHost reports whether an MCP endpoint hostname is local to the process host.
//
// Parameters:
//   - host: The normalized URL hostname is checked without a port.
//
// Return values:
//   - bool: True is returned for localhost or an IP loopback address.
func isLoopbackMCPHost(host string) bool {
\tif strings.EqualFold(strings.TrimSpace(host), "localhost") {
\t\treturn true
\t}
\tip := net.ParseIP(strings.TrimSpace(host))
\treturn ip != nil && ip.IsLoopback()
}
'''
    return source


update('model/mcp_server.go', fix_server_transport)

transport_security = r'''package mcp

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
)

// httpClient returns an HTTP client that enforces MCP credential and redirect transport rules.
//
// Parameters: none.
//
// Return values:
//   - *http.Client: The client uses the configured timeout and rejects unsafe credential redirects.
func (c *StreamableHTTPClient) httpClient() *http.Client {
	client := &http.Client{Timeout: c.Timeout}
	headers := c.headerSnapshot()
	credentialed := hasSensitiveMCPClientHeaders(headers)
	baseURL, _ := url.Parse(c.BaseURL)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.WithStack(errors.New("stopped after 10 MCP redirects"))
		}
		if len(via) == 0 {
			return nil
		}
		if strings.EqualFold(via[0].URL.Scheme, "https") && !strings.EqualFold(request.URL.Scheme, "https") {
			return errors.WithStack(errors.New("MCP redirect would downgrade HTTPS to plaintext HTTP"))
		}
		if credentialed {
			if !strings.EqualFold(request.URL.Scheme, "https") {
				return errors.WithStack(errors.New("credentialed MCP redirect must use HTTPS"))
			}
			if baseURL != nil && !sameMCPOrigin(baseURL, request.URL) {
				return errors.WithStack(errors.New("credentialed MCP redirect must preserve the endpoint origin"))
			}
		}
		return nil
	}
	return client
}

// validateTransportSecurity rejects a credentialed plaintext endpoint before any request is sent.
//
// Parameters: none.
//
// Return values:
//   - error: A stack-aware validation error is returned for non-loopback plaintext credential transport.
func (c *StreamableHTTPClient) validateTransportSecurity() error {
	if c == nil {
		return errors.WithStack(errors.New("mcp client is nil"))
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil {
		return errors.Wrap(err, "parse mcp client base URL")
	}
	if !hasSensitiveMCPClientHeaders(c.headerSnapshot()) || strings.EqualFold(parsed.Scheme, "https") || isLoopbackMCPURL(parsed) {
		return nil
	}
	return errors.WithStack(errors.New("credentialed MCP endpoint must use HTTPS unless it targets loopback"))
}

// hasSensitiveMCPClientHeaders reports whether an outbound header set contains credentials.
//
// Parameters:
//   - headers: The configured outbound header values are inspected by field name.
//
// Return values:
//   - bool: True is returned when a field name is commonly used for credentials or secrets.
func hasSensitiveMCPClientHeaders(headers map[string]string) bool {
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(key))
		for _, token := range []string{"authorization", "api-key", "apikey", "token", "secret", "password", "cookie"} {
			if strings.Contains(lower, token) {
				return true
			}
		}
	}
	return false
}

// sameMCPOrigin reports whether two URLs share scheme, hostname, and effective port.
//
// Parameters:
//   - left: The configured MCP endpoint URL provides the expected origin.
//   - right: The redirect destination URL is compared with the expected origin.
//
// Return values:
//   - bool: True is returned only when the complete origins are equal case-insensitively.
func sameMCPOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Hostname(), right.Hostname()) && effectiveMCPPort(left) == effectiveMCPPort(right)
}

// effectiveMCPPort returns the explicit or scheme-default port for an endpoint URL.
//
// Parameters:
//   - endpoint: The endpoint URL supplies the scheme and optional explicit port.
//
// Return values:
//   - string: The explicit port, the HTTP/HTTPS default, or an empty value for another scheme.
func effectiveMCPPort(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	if port := endpoint.Port(); port != "" {
		return port
	}
	switch strings.ToLower(endpoint.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// isLoopbackMCPURL reports whether an endpoint URL resolves syntactically to localhost.
//
// Parameters:
//   - endpoint: The parsed endpoint URL supplies the hostname.
//
// Return values:
//   - bool: True is returned for localhost and loopback IP literals.
func isLoopbackMCPURL(endpoint *url.URL) bool {
	if endpoint == nil {
		return false
	}
	host := strings.TrimSpace(endpoint.Hostname())
	if strings.EqualFold(host, "localhost") {
		return true
	}
	parsed := net.ParseIP(host)
	return parsed != nil && parsed.IsLoopback()
}
'''
(ROOT / 'relay/mcp/transport_security.go').write_text(transport_security)


def fix_http_clients(source: str, modern: bool) -> str:
    source = source.replace('&http.Client{Timeout: c.Timeout}', 'c.httpClient()')
    if modern:
        source = source.replace(
            '\tclient := c.httpClient()\n\tresp, err := client.Do(req)',
            '\tif err := c.validateTransportSecurity(); err != nil {\n\t\treturn err\n\t}\n\tclient := c.httpClient()\n\tresp, err := client.Do(req)',
        )
    else:
        positions = []
        start = 0
        while True:
            index = source.find('\tclient := c.httpClient()\n\tresp, err := client.Do(req)', start)
            if index < 0:
                break
            positions.append(index)
            start = index + 1
        for index in reversed(positions):
            function_start = source.rfind('\nfunc ', 0, index)
            function_header_end = source.find('{', function_start)
            header = source[function_start:function_header_end]
            replacement = '\tif err := c.validateTransportSecurity(); err != nil {\n'
            if header.rstrip().endswith(' error)') or header.rstrip().endswith(' error'):
                replacement += '\t\treturn err\n'
            else:
                replacement += '\t\treturn nil, err\n'
            replacement += '\t}\n\tclient := c.httpClient()\n\tresp, err := client.Do(req)'
            source = source[:index] + replacement + source[index + len('\tclient := c.httpClient()\n\tresp, err := client.Do(req)'):]
    return source


update('relay/mcp/client.go', lambda source: fix_http_clients(source, False))
update('relay/mcp/client_latest.go', lambda source: fix_http_clients(source, True))

end_to_end_test = r'''package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestCallToolLatestNormalizesNilArguments verifies zero-argument modern calls transmit a JSON object instead of null.
//
// Parameters:
//   - t: The test context owns the mock MCP server and assertions.
//
// Return values: none; failures are reported through t.
func TestCallToolLatestNormalizesNilArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			ID     any `json:"id"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
		require.NotNil(t, envelope.Params.Arguments)
		require.Empty(t, envelope.Params.Arguments)
		writer.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{"resultType": ResultTypeComplete, "content": []any{}},
		}))
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	result, err := client.CallToolLatestWithDescriptor(context.Background(), ToolDescriptor{
		Name:        "health",
		InputSchema: map[string]any{"type": "object", "additionalProperties": false},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, ResultTypeComplete, result.ResultType)
}

// TestMCPClientTransportSecurity verifies credentialed plaintext and redirect downgrade behavior.
//
// Parameters:
//   - t: The test context owns transport policy assertions.
//
// Return values: none; failures are reported through t.
func TestMCPClientTransportSecurity(t *testing.T) {
	t.Run("reject remote plaintext credentials", func(t *testing.T) {
		client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: "http://example.com/mcp"}, map[string]string{"Authorization": "Bearer secret"}, time.Second)
		require.ErrorContains(t, client.validateTransportSecurity(), "HTTPS")
	})
	t.Run("allow loopback plaintext credentials", func(t *testing.T) {
		client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: "http://127.0.0.1:8080/mcp"}, map[string]string{"Authorization": "Bearer secret"}, time.Second)
		require.NoError(t, client.validateTransportSecurity())
	})
	t.Run("reject credentialed cross-origin redirect", func(t *testing.T) {
		client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: "https://example.com/mcp"}, map[string]string{"X-API-Key": "secret"}, time.Second)
		initial := &http.Request{URL: mustMCPTestURL(t, "https://example.com/mcp")}
		next := &http.Request{URL: mustMCPTestURL(t, "https://other.example/mcp")}
		require.ErrorContains(t, client.httpClient().CheckRedirect(next, []*http.Request{initial}), "origin")
	})
	t.Run("reject HTTPS downgrade redirect", func(t *testing.T) {
		client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: "https://example.com/mcp"}, nil, time.Second)
		initial := &http.Request{URL: mustMCPTestURL(t, "https://example.com/mcp")}
		next := &http.Request{URL: mustMCPTestURL(t, "http://example.com/mcp")}
		require.ErrorContains(t, client.httpClient().CheckRedirect(next, []*http.Request{initial}), "downgrade")
	})
}

// TestMCPOptionalNullWireFields verifies explicit null optional fields remain compatible while malformed values fail.
//
// Parameters:
//   - t: The test context owns JSON compatibility assertions.
//
// Return values: none; failures are reported through t.
func TestMCPOptionalNullWireFields(t *testing.T) {
	var tool ToolDescriptor
	require.NoError(t, json.Unmarshal([]byte(`{"name":"echo","inputSchema":{"type":"object"},"outputSchema":null,"annotations":null,"icons":null,"_meta":null}`), &tool))
	require.Nil(t, tool.OutputSchema)
	require.Nil(t, tool.Annotations)
	require.Nil(t, tool.Icons)
	require.Nil(t, tool.Meta)
	var result CallToolResult
	require.NoError(t, json.Unmarshal([]byte(`{"resultType":"complete","content":[],"structuredContent":null,"inputRequests":null,"requestState":null,"_meta":null}`), &result))
	require.Nil(t, result.StructuredContent)
	require.Nil(t, result.InputRequests)
	require.Empty(t, result.RequestState)
	require.Nil(t, result.Meta)
	require.Error(t, json.Unmarshal([]byte(`{"name":"echo","inputSchema":{"type":"object"},"annotations":42}`), &tool))
}

// mustMCPTestURL parses a fixed test URL and reports impossible fixture errors through the test context.
//
// Parameters:
//   - t: The test context receives parse failures.
//   - raw: The fixed absolute test URL is parsed.
//
// Return values:
//   - *url.URL: The parsed URL is returned for request construction.
func mustMCPTestURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}
'''
(ROOT / 'relay/mcp/end_to_end_protocol_test.go').write_text(end_to_end_test)

model_test = r'''package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMCPServerCredentialTransportValidation verifies credentialed remote endpoints require HTTPS without blocking loopback development.
//
// Parameters:
//   - t: The test context owns server configuration validation assertions.
//
// Return values: none; failures are reported through t.
func TestMCPServerCredentialTransportValidation(t *testing.T) {
	t.Run("reject remote plaintext bearer credentials", func(t *testing.T) {
		server := &MCPServer{Name: "remote", BaseURL: "http://example.com/mcp", AuthType: MCPAuthTypeBearer, APIKey: "secret"}
		require.ErrorContains(t, server.NormalizeAndValidate(), "https")
	})
	t.Run("allow remote HTTPS bearer credentials", func(t *testing.T) {
		server := &MCPServer{Name: "remote", BaseURL: "https://example.com/mcp", AuthType: MCPAuthTypeBearer, APIKey: "secret"}
		require.NoError(t, server.NormalizeAndValidate())
	})
	t.Run("allow loopback plaintext bearer credentials", func(t *testing.T) {
		server := &MCPServer{Name: "local", BaseURL: "http://localhost:3000/mcp", AuthType: MCPAuthTypeBearer, APIKey: "secret"}
		require.NoError(t, server.NormalizeAndValidate())
	})
	t.Run("reject remote plaintext sensitive custom header", func(t *testing.T) {
		server := &MCPServer{Name: "custom", BaseURL: "http://example.com/mcp", Headers: JSONStringMap{"X-API-Key": "secret"}}
		require.ErrorContains(t, server.NormalizeAndValidate(), "https")
	})
}
'''
(ROOT / 'model/mcp_server_transport_test.go').write_text(model_test)

controller_test = r'''package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/mcp"
)

// TestMCPProxyLatestAcceptsLegacyInitializeWithoutParams verifies compatibility with clients that omit optional initialize parameters.
//
// Parameters:
//   - t: The test context owns the in-memory HTTP request and assertions.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestAcceptsLegacyInitializeWithoutParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)
	body := []byte(`{"jsonrpc":"2.0","id":"legacy-no-params","method":"initialize"}`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, mcp.LegacyProtocolVersion, envelope.Result.ProtocolVersion)
}

// TestMCPProxyLatestPreservesInvalidCursorStatus verifies method validation errors are not rewritten as internal errors.
//
// Parameters:
//   - t: The test context owns the in-memory modern MCP request and assertions.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestPreservesInvalidCursorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)
	body := modernMCPRequestBody(t, "bad-cursor", "tools/list", map[string]any{"cursor": "%%%"})
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	request.Header.Set(mcp.MethodHeader, "tools/list")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	var envelope struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, mcpErrInvalidParams, envelope.Error.Code)
}

// TestMCPProxyLatestRejectsModernGETAndDELETE verifies the stateless 2026-07-28 HTTP profile exposes POST only.
//
// Parameters:
//   - t: The test context owns modern transport method assertions.
//
// Return values: none; failures are reported through t.
func TestMCPProxyLatestRejectsModernGETAndDELETE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			router := gin.New()
			router.Any("/mcp", MCPProxyLatest)
			request := httptest.NewRequest(method, "/mcp", nil)
			request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusMethodNotAllowed, response.Code)
			require.Equal(t, http.MethodPost, response.Header().Get("Allow"))
		})
	}
}
'''
(ROOT / 'controller/mcp_protocol_behavior_test.go').write_text(controller_test)
