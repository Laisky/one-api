package mcp

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	// ProtocolVersion is the latest stable MCP protocol version supported by one-api.
	ProtocolVersion = "2026-07-28"
	// LegacyProtocolVersion is the preferred initialization-based MCP protocol version.
	LegacyProtocolVersion = "2025-11-25"
	// LegacyProtocolVersionFallback keeps compatibility with older Streamable HTTP servers.
	LegacyProtocolVersionFallback = "2025-06-18"

	// ProtocolVersionHeader carries the protocol version for every modern request.
	ProtocolVersionHeader = "Mcp-Protocol-Version"
	// MethodHeader mirrors the JSON-RPC method for modern request validation.
	MethodHeader = "Mcp-Method"
	// NameHeader mirrors the named resource or tool for modern request validation.
	NameHeader = "Mcp-Name"
	// ParameterHeaderPrefix prefixes schema-driven argument headers.
	ParameterHeaderPrefix = "Mcp-Param-"
	// SessionIDHeader carries legacy Streamable HTTP session identifiers.
	SessionIDHeader = "Mcp-Session-Id"

	// MetaProtocolVersionKey identifies the request protocol version in _meta.
	MetaProtocolVersionKey = "io.modelcontextprotocol/protocolVersion"
	// MetaClientInfoKey identifies the client implementation in _meta.
	MetaClientInfoKey = "io.modelcontextprotocol/clientInfo"
	// MetaClientCapabilitiesKey identifies per-request client capabilities in _meta.
	MetaClientCapabilitiesKey = "io.modelcontextprotocol/clientCapabilities"
	// MetaServerInfoKey identifies the server implementation in result _meta.
	MetaServerInfoKey = "io.modelcontextprotocol/serverInfo"

	// ResultTypeComplete marks a final successful result.
	ResultTypeComplete = "complete"
	// ResultTypeInputRequired marks a result that requires additional client input.
	ResultTypeInputRequired = "input_required"

	// CacheScopePublic allows a cacheable result to be reused between users.
	CacheScopePublic = "public"
	// CacheScopePrivate restricts a cacheable result to the authenticated user.
	CacheScopePrivate = "private"

	// ErrorCodeHeaderMismatch reports disagreement between JSON fields and mirrored headers.
	ErrorCodeHeaderMismatch = -32020
	// ErrorCodeMissingRequiredClientCapability reports an undeclared required capability.
	ErrorCodeMissingRequiredClientCapability = -32021
	// ErrorCodeUnsupportedProtocolVersion reports an unsupported modern protocol version.
	ErrorCodeUnsupportedProtocolVersion = -32022
)

// ImplementationInfo identifies an MCP client or server implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// RequestMeta contains the namespaced per-request metadata required by MCP 2026-07-28.
type RequestMeta struct {
	ProtocolVersion    string             `json:"io.modelcontextprotocol/protocolVersion"`
	ClientInfo         ImplementationInfo `json:"io.modelcontextprotocol/clientInfo"`
	ClientCapabilities map[string]any     `json:"io.modelcontextprotocol/clientCapabilities"`
}

// ResponseMeta contains the namespaced server identity returned with modern MCP results.
type ResponseMeta struct {
	ServerInfo ImplementationInfo `json:"io.modelcontextprotocol/serverInfo"`
}

// DiscoveryResult describes protocol versions and capabilities exposed by an MCP server.
type DiscoveryResult struct {
	ResultType        string         `json:"resultType"`
	SupportedVersions []string       `json:"supportedVersions"`
	Capabilities      map[string]any `json:"capabilities"`
	Instructions      string         `json:"instructions,omitempty"`
	TTLMS             int64          `json:"ttlMs"`
	CacheScope        string         `json:"cacheScope"`
	Meta              ResponseMeta   `json:"_meta"`
}

// ProtocolError preserves HTTP and JSON-RPC error details for negotiation decisions.
type ProtocolError struct {
	HTTPStatus int
	Code       int
	Message    string
	Data       any
	Body       string
}

// Error renders ProtocolError without discarding transport or JSON-RPC context.
//
// Parameters: none.
//
// Return values:
//   - string: a stable diagnostic string containing the available status, code, and message.
func (e *ProtocolError) Error() string {
	if e == nil {
		return "mcp protocol error"
	}
	parts := make([]string, 0, 3)
	if e.HTTPStatus != 0 {
		parts = append(parts, fmt.Sprintf("status %d", e.HTTPStatus))
	}
	if e.Code != 0 {
		parts = append(parts, fmt.Sprintf("code %d", e.Code))
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = strings.TrimSpace(e.Body)
	}
	if message != "" {
		parts = append(parts, message)
	}
	if len(parts) == 0 {
		return "mcp protocol error"
	}
	return "mcp protocol error: " + strings.Join(parts, ": ")
}

// SupportedProtocolVersions returns protocol versions that the gateway can serve.
//
// Parameters: none.
//
// Return values:
//   - []string: a new slice ordered from the preferred modern version to legacy compatibility versions.
func SupportedProtocolVersions() []string {
	return []string{ProtocolVersion, LegacyProtocolVersion, LegacyProtocolVersionFallback}
}

// IsLegacyProtocolVersion reports whether version selects the initialization-based protocol era.
//
// Parameters:
//   - version: the protocol version supplied by an MCP peer or HTTP header.
//
// Return values:
//   - bool: true only for legacy versions that one-api explicitly supports.
func IsLegacyProtocolVersion(version string) bool {
	switch strings.TrimSpace(version) {
	case LegacyProtocolVersion, LegacyProtocolVersionFallback:
		return true
	default:
		return false
	}
}

// IsSupportedProtocolVersion reports whether version is supported by either MCP protocol era.
//
// Parameters:
//   - version: the protocol version supplied by an MCP peer.
//
// Return values:
//   - bool: true when one-api can process the version through a modern or legacy path.
func IsSupportedProtocolVersion(version string) bool {
	return strings.TrimSpace(version) == ProtocolVersion || IsLegacyProtocolVersion(version)
}

// NegotiateLegacyProtocolVersion chooses the legacy protocol version returned by initialize.
//
// Parameters:
//   - requested: the version requested in legacy initialize parameters.
//
// Return values:
//   - string: the requested version when supported, otherwise the preferred legacy version.
func NegotiateLegacyProtocolVersion(requested string) string {
	requested = strings.TrimSpace(requested)
	if IsLegacyProtocolVersion(requested) {
		return requested
	}
	return LegacyProtocolVersion
}

// ModernRequestMeta returns the metadata attached to each MCP 2026-07-28 request.
//
// Parameters: none.
//
// Return values:
//   - RequestMeta: fresh request metadata with an explicit empty optional-capability set.
func ModernRequestMeta() RequestMeta {
	return RequestMeta{
		ProtocolVersion: ProtocolVersion,
		ClientInfo: ImplementationInfo{
			Name:    mcpClientName,
			Version: mcpClientVersion,
		},
		ClientCapabilities: map[string]any{},
	}
}

// ServerResponseMeta returns one-api's server identity for modern results.
//
// Parameters:
//   - name: the public server implementation name.
//   - version: the public server implementation version.
//
// Return values:
//   - ResponseMeta: metadata containing the supplied server identity.
func ServerResponseMeta(name, version string) ResponseMeta {
	return ResponseMeta{ServerInfo: ImplementationInfo{Name: name, Version: version}}
}

// WithModernMeta returns object parameters containing the required modern request metadata.
//
// Parameters:
//   - params: method-specific parameters; nil is treated as an empty object.
//
// Return values:
//   - map[string]any: a new parameters object that does not mutate the caller's map.
func WithModernMeta(params map[string]any) map[string]any {
	out := make(map[string]any, len(params)+1)
	for key, value := range params {
		out[key] = value
	}
	out["_meta"] = ModernRequestMeta()
	return out
}

// IsRecognizedModernError reports whether an error proves that the peer speaks modern MCP.
//
// Parameters:
//   - err: the transport or JSON-RPC error returned by a modern request.
//
// Return values:
//   - bool: true when retrying through the legacy handshake would be incorrect.
func IsRecognizedModernError(err error) bool {
	var protocolErr *ProtocolError
	if !stderrors.As(err, &protocolErr) || protocolErr == nil {
		return false
	}
	switch protocolErr.Code {
	case ErrorCodeHeaderMismatch, ErrorCodeMissingRequiredClientCapability, ErrorCodeUnsupportedProtocolVersion:
		return true
	case -32601:
		return protocolErr.HTTPStatus == http.StatusNotFound
	default:
		return false
	}
}

// IsModernFallbackCandidate reports whether a failed modern request should retry through the legacy handshake.
//
// Parameters:
//   - err: the error returned by a modern request attempt.
//
// Return values:
//   - bool: true only for failures that plausibly indicate a legacy endpoint.
func IsModernFallbackCandidate(err error) bool {
	var protocolErr *ProtocolError
	if !stderrors.As(err, &protocolErr) || protocolErr == nil {
		return false
	}
	if protocolErr.HTTPStatus == http.StatusUnauthorized || protocolErr.HTTPStatus == http.StatusForbidden {
		return false
	}
	if IsRecognizedModernError(err) {
		return false
	}
	if protocolErr.HTTPStatus == http.StatusBadRequest || protocolErr.HTTPStatus == http.StatusNotFound || protocolErr.HTTPStatus == http.StatusMethodNotAllowed {
		return true
	}
	message := strings.ToLower(protocolErr.Message + " " + protocolErr.Body)
	if protocolErr.Code == -32002 || protocolErr.Code == -32600 || protocolErr.Code == -32601 {
		return strings.Contains(message, "initial") || strings.Contains(message, "session") || strings.Contains(message, "protocol")
	}
	return false
}
