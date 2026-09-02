package controller

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/mcp"
)

const (
	modernMCPMaxRequestBytes int64 = 4 << 20
	legacyMCPMaxRequestBytes int64 = 32 << 20
)

// modernMCPRequestMeta extracts the required per-request metadata for MCP 2026-07-28.
type modernMCPRequestMeta struct {
	ProtocolVersion    string                  `json:"io.modelcontextprotocol/protocolVersion"`
	ClientInfo         *mcp.ImplementationInfo `json:"io.modelcontextprotocol/clientInfo,omitempty"`
	ClientCapabilities map[string]any          `json:"io.modelcontextprotocol/clientCapabilities"`
}

// modernMCPParamsEnvelope extracts modern metadata without constraining method-specific parameters.
type modernMCPParamsEnvelope struct {
	Meta modernMCPRequestMeta `json:"_meta"`
}

// modernMCPCallParams contains one tools/call request and optional multi-round-trip state.
type modernMCPCallParams struct {
	Name           string         `json:"name"`
	Arguments      map[string]any `json:"arguments"`
	Signature      string         `json:"signature,omitempty"`
	InputResponses map[string]any `json:"inputResponses,omitempty"`
	RequestState   string         `json:"requestState,omitempty"`
}

// modernMCPValidationError carries HTTP and JSON-RPC details for one rejected modern request.
type modernMCPValidationError struct {
	Status int
	Code   int
	Err    error
	Data   any
}

// replayReadCloser replays bytes already inspected while retaining ownership of the original request body.
type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

// Error returns the underlying modern request validation message.
//
// Parameters: none.
//
// Return values:
//   - string: the validation message or a stable fallback when the receiver is incomplete.
func (e *modernMCPValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "invalid modern mcp request"
	}
	return e.Err.Error()
}

// Unwrap returns the underlying validation error for errors.Is and errors.As.
//
// Parameters: none.
//
// Return values:
//   - error: The underlying validation error is returned, or nil for an empty receiver.
func (e *modernMCPValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Close closes the original HTTP request body retained by replayReadCloser.
//
// Parameters: none.
//
// Return values:
//   - error: the original request body close error, or nil when no closer is present.
func (r *replayReadCloser) Close() error {
	if r == nil || r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

// MCPProxyLatest dispatches modern MCP requests and delegates recognized legacy traffic unchanged.
//
// Parameters:
//   - c: the Gin request context containing the authenticated Streamable HTTP request.
//
// Return values: none; the function writes the complete HTTP response or delegates to MCPProxy.
func MCPProxyLatest(c *gin.Context) {
	if err := validateModernMCPOrigin(c.Request); err != nil {
		respondMCPModernError(c, nil, http.StatusForbidden, mcpErrInvalidRequest, err, nil)
		return
	}

	versionValues := c.Request.Header.Values(mcp.ProtocolVersionHeader)
	if len(versionValues) > 1 {
		respondMCPModernError(c, nil, http.StatusBadRequest, mcp.ErrorCodeHeaderMismatch, errors.Errorf("%s must occur at most once", mcp.ProtocolVersionHeader), nil)
		return
	}
	modernTransport := len(versionValues) == 1 && !mcp.IsLegacyProtocolVersion(versionValues[0])

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

	requestLimit := legacyMCPMaxRequestBytes
	if modernTransport {
		requestLimit = modernMCPMaxRequestBytes
	}
	body, err := readBoundedMCPRequestBody(c.Request.Body, requestLimit)
	if err != nil {
		var tooLarge *mcpRequestTooLargeError
		if stderrors.As(err, &tooLarge) {
			respondMCPModernError(c, nil, http.StatusRequestEntityTooLarge, mcpErrInvalidRequest, err, nil)
			return
		}
		respondMCPModernError(c, nil, http.StatusBadRequest, mcpErrParseError, errors.Wrap(err, "read mcp request"), nil)
		return
	}
	originalBody := c.Request.Body
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
}

// mcpRequestTooLargeError records the configured request-body limit that was exceeded.
type mcpRequestTooLargeError struct {
	Limit int64
}

// Error returns a stable request-body limit message.
//
// Parameters: none.
//
// Return values:
//   - string: The configured byte limit is included in the message.
func (e *mcpRequestTooLargeError) Error() string {
	if e == nil {
		return "mcp request body is too large"
	}
	return fmt.Sprintf("mcp request body exceeds %d bytes", e.Limit)
}

// readBoundedMCPRequestBody reads one complete request without exceeding a fixed allocation boundary.
//
// Parameters:
//   - reader: The inbound request body supplies the bytes to consume.
//   - limit: The maximum accepted body size is expressed in bytes.
//
// Return values:
//   - []byte: The complete request body is returned when it is within the limit.
//   - error: A wrapped read error or mcpRequestTooLargeError is returned on failure.
func readBoundedMCPRequestBody(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, errors.WithStack(errors.New("mcp request body is nil"))
	}
	if limit < 0 {
		return nil, errors.WithStack(errors.New("mcp request body limit is negative"))
	}
	limited := &io.LimitedReader{R: reader, N: limit}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, errors.Wrap(err, "read bounded mcp request body")
	}
	if limited.N == 0 {
		var extra [1]byte
		_, probeErr := io.ReadFull(reader, extra[:])
		if probeErr == nil {
			return nil, &mcpRequestTooLargeError{Limit: limit}
		}
		if !stderrors.Is(probeErr, io.EOF) {
			return nil, errors.Wrap(probeErr, "probe bounded mcp request body")
		}
	}
	return body, nil
}

// isModernMCPRequest reports whether a request selects the 2026-07-28 stateless protocol profile.
//
// Parameters:
//   - c: the Gin request context containing transport headers.
//   - request: the parsed JSON-RPC request.
//
// Return values:
//   - bool: true for modern metadata, the modern version header, discovery, or an unknown non-legacy version.
func isModernMCPRequest(c *gin.Context, request mcpRPCRequest) bool {
	if strings.TrimSpace(request.Method) == "server/discover" {
		return true
	}
	var params modernMCPParamsEnvelope
	if json.Unmarshal(request.Params, &params) == nil && strings.TrimSpace(params.Meta.ProtocolVersion) != "" {
		return true
	}
	versions := c.Request.Header.Values(mcp.ProtocolVersionHeader)
	if len(versions) != 1 {
		return len(versions) > 1
	}
	version := strings.TrimSpace(versions[0])
	return version != "" && !mcp.IsLegacyProtocolVersion(version)
}

// validateModernMCPRequest validates JSON-RPC identity, metadata, and mirrored transport headers.
//
// Parameters:
//   - c: the Gin request context containing transport headers.
//   - request: the parsed modern JSON-RPC request.
//
// Return values:
//   - error: a modernMCPValidationError describing the required HTTP status and JSON-RPC code.
func validateModernMCPRequest(c *gin.Context, request mcpRPCRequest) error {
	if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("jsonrpc must be 2.0 and method is required")}
	}
	if request.ID == nil && isModernMCPRequestMethod(request.Method) {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("modern mcp requests require a non-null id")}
	}
	if request.ID != nil && !isValidModernMCPRequestID(request.ID) {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("modern mcp request id must be a string or integer")}
	}

	var params modernMCPParamsEnvelope
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.Wrap(err, "decode modern mcp metadata")}
	}
	bodyVersion := strings.TrimSpace(params.Meta.ProtocolVersion)
	headerVersion, err := singleMCPHeaderValue(c.Request.Header, mcp.ProtocolVersionHeader)
	if err != nil || bodyVersion == "" {
		if err == nil {
			err = errors.New("modern mcp requests require protocol version metadata")
		}
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	if bodyVersion != headerVersion {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: errors.New("MCP-Protocol-Version does not match request _meta")}
	}
	if bodyVersion != mcp.ProtocolVersion {
		return &modernMCPValidationError{
			Status: http.StatusBadRequest,
			Code:   mcp.ErrorCodeUnsupportedProtocolVersion,
			Err:    errors.Errorf("unsupported mcp protocol version %q", bodyVersion),
			Data: gin.H{
				"supported": mcp.SupportedProtocolVersions(),
				"requested": bodyVersion,
			},
		}
	}
	if params.Meta.ClientCapabilities == nil {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("modern mcp requests require client capabilities in _meta")}
	}
	headerMethod, err := singleMCPHeaderValue(c.Request.Header, mcp.MethodHeader)
	if err != nil || headerMethod != request.Method {
		if err == nil {
			err = errors.New("MCP-Method does not match the JSON-RPC method")
		}
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	if request.Method != "tools/call" {
		return nil
	}

	var callParams modernMCPCallParams
	if err := json.Unmarshal(request.Params, &callParams); err != nil {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidParams, Err: errors.Wrap(err, "decode mcp call params")}
	}
	headerName, err := singleMCPHeaderValue(c.Request.Header, mcp.NameHeader)
	if err != nil {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	decodedName, err := mcp.DecodeMCPHeaderValue(headerName)
	if err != nil || decodedName != callParams.Name {
		if err == nil {
			err = errors.New("MCP-Name does not match tools/call params.name")
		}
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	return nil
}

// isModernMCPRequestMethod reports whether the tools-only modern endpoint expects a response identifier.
//
// Parameters:
//   - method: the JSON-RPC method to classify.
//
// Return values:
//   - bool: true for discovery, tool listing, and tool execution requests.
func isModernMCPRequestMethod(method string) bool {
	switch method {
	case "server/discover", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}

// isValidModernMCPRequestID reports whether a decoded JSON-RPC identifier is a string or integer.
//
// Parameters:
//   - id: the JSON-decoded request identifier.
//
// Return values:
//   - bool: true for strings and finite integral JSON numbers.
func isValidModernMCPRequestID(id any) bool {
	switch typed := id.(type) {
	case string:
		return true
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0) && math.Trunc(typed) == typed
	default:
		return false
	}
}

// singleMCPHeaderValue returns one required non-empty header value.
//
// Parameters:
//   - headers: the HTTP request headers.
//   - name: the case-insensitive header field name.
//
// Return values:
//   - string: the sole non-empty value.
//   - error: a cardinality or empty-value error.
func singleMCPHeaderValue(headers http.Header, name string) (string, error) {
	values := headers.Values(name)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", errors.Errorf("%s must occur exactly once", name)
	}
	return values[0], nil
}

// validateModernMCPOrigin protects browser-accessible HTTP endpoints from DNS rebinding.
//
// Parameters:
//   - request: the inbound HTTP request.
//
// Return values:
//   - error: a validation error when a present Origin is malformed or targets another host.
func validateModernMCPOrigin(request *http.Request) error {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("invalid Origin header")
	}
	if !strings.EqualFold(parsed.Host, strings.TrimSpace(request.Host)) {
		return errors.Errorf("Origin host %q does not match the MCP endpoint host", parsed.Host)
	}
	return nil
}

// respondModernValidationError writes one modern protocol validation failure.
//
// Parameters:
//   - c: the Gin request context receiving the response.
//   - id: the decoded JSON-RPC request identifier.
//   - err: a modernMCPValidationError or an unexpected validation error.
//
// Return values: none; the function writes a complete JSON-RPC error response.
func respondModernValidationError(c *gin.Context, id any, err error) {
	var validationErr *modernMCPValidationError
	if !stderrors.As(err, &validationErr) || validationErr == nil {
		respondMCPModernError(c, id, http.StatusBadRequest, mcpErrInvalidRequest, err, nil)
		return
	}
	respondMCPModernError(c, id, validationErr.Status, validationErr.Code, validationErr.Err, validationErr.Data)
}

// handleModernMCPPost serves the tools-only MCP 2026-07-28 method surface.
//
// Parameters:
//   - c: the Gin request context containing authentication and request-scoped logging.
//   - request: the validated modern JSON-RPC request.
//
// Return values: none; the function writes a complete JSON-RPC response.
func handleModernMCPPost(c *gin.Context, request mcpRPCRequest) {
	switch strings.TrimSpace(request.Method) {
	case "server/discover":
		respondMCPModernResult(c, request.ID, mcp.DiscoveryResult{
			ResultType:        mcp.ResultTypeComplete,
			SupportedVersions: mcp.SupportedProtocolVersions(),
			Capabilities: gin.H{
				"tools": gin.H{"listChanged": false},
			},
			TTLMS:      3600000,
			CacheScope: mcp.CacheScopePrivate,
			Meta:       mcp.ServerResponseMeta(mcpServerName, mcpServerVersion),
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

// respondModernDispatchError preserves modern validation status while redacting unexpected internal failures.
//
// Parameters:
//   - c: The Gin request context receives the JSON-RPC response.
//   - id: The JSON-RPC request identifier is reflected in the response.
//   - err: The method failure is classified as a validation or internal error.
//
// Return values: none; the function writes the complete JSON-RPC error response.
func respondModernDispatchError(c *gin.Context, id any, err error) {
	var validationErr *modernMCPValidationError
	if stderrors.As(err, &validationErr) && validationErr != nil {
		respondModernValidationError(c, id, validationErr)
		return
	}
	respondMCPModernError(c, id, http.StatusOK, mcpErrInternal, err, nil)
}

// respondMCPModernResult writes a successful JSON-RPC result with required defaults and server identity.
//
// Parameters:
//   - c: the Gin request context receiving the response.
//   - id: the decoded JSON-RPC request identifier.
//   - result: the result object to normalize and encode.
//
// Return values: none; the function writes a complete JSON-RPC response.
func respondMCPModernResult(c *gin.Context, id any, result any) {
	encoded, err := json.Marshal(result)
	if err != nil {
		respondMCPModernError(c, id, http.StatusOK, mcpErrInternal, errors.Wrap(err, "marshal modern mcp result"), nil)
		return
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		respondMCPModernError(c, id, http.StatusOK, mcpErrInternal, errors.Wrap(err, "normalize modern mcp result"), nil)
		return
	}
	if normalized["resultType"] == nil || normalized["resultType"] == "" {
		normalized["resultType"] = mcp.ResultTypeComplete
	}
	if content, exists := normalized["content"]; exists && content == nil {
		delete(normalized, "content")
	}
	promoteModernResultAlias(normalized, "structured_content", "structuredContent")
	promoteModernResultAlias(normalized, "is_error", "isError")
	promoteModernResultAlias(normalized, "input_requests", "inputRequests")
	promoteModernResultAlias(normalized, "request_state", "requestState")
	meta, _ := normalized["_meta"].(map[string]any)
	if meta == nil {
		meta = make(map[string]any)
	}
	meta[mcp.MetaServerInfoKey] = gin.H{"name": mcpServerName, "version": mcpServerVersion}
	normalized["_meta"] = meta
	c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "id": id, "result": normalized})
}

// promoteModernResultAlias moves one legacy result field to its current camelCase name.
//
// Parameters:
//   - result: the mutable result object.
//   - legacyName: the legacy snake_case field name.
//   - modernName: the current camelCase field name.
//
// Return values: none; result is updated in place.
func promoteModernResultAlias(result map[string]any, legacyName, modernName string) {
	if value, exists := result[legacyName]; exists {
		if _, modernExists := result[modernName]; !modernExists {
			result[modernName] = value
		}
		delete(result, legacyName)
	}
}

// respondMCPModernError writes one JSON-RPC error while redacting internal implementation details.
//
// Parameters:
//   - c: the Gin request context receiving the response and providing the request-scoped logger.
//   - id: the decoded JSON-RPC request identifier, which may be nil for parse failures.
//   - status: the HTTP status required by the transport profile.
//   - code: the JSON-RPC or MCP error code.
//   - err: the underlying validation or internal error.
//   - data: optional protocol-safe structured error details.
//
// Return values: none; the function writes a complete JSON-RPC error response.
func respondMCPModernError(c *gin.Context, id any, status int, code int, err error, data any) {
	message := "mcp request failed"
	if code == mcpErrInternal {
		logger := gmw.GetLogger(c)
		if err != nil {
			logger.Error("mcp internal request failure", zap.Error(err))
		}
		message = "internal MCP error"
	} else if err != nil {
		message = err.Error()
	}
	errorObject := gin.H{"code": code, "message": message}
	if data != nil {
		errorObject["data"] = data
	}
	c.JSON(status, gin.H{"jsonrpc": "2.0", "id": id, "error": errorObject})
}
