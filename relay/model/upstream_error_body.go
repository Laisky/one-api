package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/Laisky/errors/v2"
)

// This file holds the client-visible policy for upstream error bodies. Provider
// bodies are untrusted: they can carry internal hosts, account identifiers and
// other data that must not reach API callers (OWASP API8:2023 security
// misconfiguration, API10:2023 unsafe consumption of APIs), and their size is
// chosen by the provider (OWASP API4:2023 unrestricted resource consumption).
// Callers therefore read at most MaxUpstreamErrorBodyBytes+1 bytes, place only
// parsed provider fields in Error.Message, and otherwise use the generic
// messages below, which describe the body without echoing any of it. Logs never
// carry bodies either; see logUpstreamResponseFromBytes in relay/controller.

// MaxUpstreamErrorBodyBytes bounds how many bytes of an upstream error body the
// gateway reads and parses. Reads are capped at this size plus one byte, and a
// body that exceeds it is reported as oversized instead of parsing a truncated
// prefix. Small upstream metadata responses (such as countTokens) share the bound.
const MaxUpstreamErrorBodyBytes = 1 << 20

// maxUpstreamContentTypeBytes bounds the provider Content-Type header that is
// parsed at all; longer headers are treated as absent.
const maxUpstreamContentTypeBytes = 256

// maxUpstreamMediaTypeBytes bounds the media type that may appear in a
// client-visible message.
const maxUpstreamMediaTypeBytes = 100

// ReadBoundedUpstreamBody reads at most MaxUpstreamErrorBodyBytes+1 bytes from
// an upstream response body. Reading one byte past the limit lets callers
// detect an over-limit body (len > MaxUpstreamErrorBodyBytes) without reading
// the rest of it. The caller remains responsible for closing the body.
//
// Parameters:
//   - body: the upstream response body; nil is treated as empty.
//
// Returns:
//   - []byte: the bytes read, at most MaxUpstreamErrorBodyBytes+1.
//   - error: a wrapped read failure; the bytes read before it are still returned.
func ReadBoundedUpstreamBody(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxUpstreamErrorBodyBytes+1))
	if err != nil {
		return data, errors.Wrap(err, "read bounded upstream body")
	}
	return data, nil
}

// UnrecognizedUpstreamErrorMessage returns the client-visible message for an
// upstream error response whose body carries no recognizable error message, or
// that is empty, oversized or not a JSON object. It never includes body bytes;
// only the status, a sanitized media type and the body size are described.
//
// Parameters:
//   - statusCode: the upstream HTTP status code.
//   - contentType: the provider's raw Content-Type header. Only a sanitized media
//     type (lowercase, restricted charset, bounded length) is echoed; an
//     unparseable or unusual value is omitted.
//   - bodyBytes: the number of body bytes read; zero (or a negative value)
//     describes an empty body.
//   - oversized: true when the body exceeded MaxUpstreamErrorBodyBytes and was not parsed.
//
// Returns:
//   - string: a bounded, generic message such as
//     "upstream returned HTTP 502 with an unrecognized error body (content-type text/html, 1234 bytes)".
func UnrecognizedUpstreamErrorMessage(statusCode int, contentType string, bodyBytes int, oversized bool) string {
	mediaType := sanitizeUpstreamMediaType(contentType)
	switch {
	case oversized && mediaType == "":
		return fmt.Sprintf("upstream returned HTTP %d with an error body larger than the %d-byte limit", statusCode, MaxUpstreamErrorBodyBytes)
	case oversized:
		return fmt.Sprintf("upstream returned HTTP %d with an error body larger than the %d-byte limit (content-type %s)", statusCode, MaxUpstreamErrorBodyBytes, mediaType)
	case bodyBytes <= 0:
		return fmt.Sprintf("upstream returned HTTP %d with an empty error body", statusCode)
	case mediaType == "":
		return fmt.Sprintf("upstream returned HTTP %d with an unrecognized error body (%d bytes)", statusCode, bodyBytes)
	default:
		return fmt.Sprintf("upstream returned HTTP %d with an unrecognized error body (content-type %s, %d bytes)", statusCode, mediaType, bodyBytes)
	}
}

// UnreadableUpstreamErrorMessage returns the client-visible message for an
// upstream error response whose body could not be read or closed. The transport
// error itself is excluded because it can carry upstream URLs; callers keep it
// in Error.RawError for diagnostics instead.
//
// Parameters:
//   - statusCode: the upstream HTTP status code.
//
// Returns:
//   - string: a generic message such as "upstream returned HTTP 502 with an error body that could not be read".
func UnreadableUpstreamErrorMessage(statusCode int) string {
	return fmt.Sprintf("upstream returned HTTP %d with an error body that could not be read", statusCode)
}

// UpstreamErrorBodyText extracts a provider's human-readable error message from
// a JSON error body without echoing anything else. It recognizes the common
// shapes {"error":"..."}, {"error":{"message":"..."}}, {"message":"..."} and the
// FastAPI {"detail":"..."}, in that order. Only JSON string values count; any
// other kind is absent. Keys match exactly. Adaptors whose provider documents a
// richer envelope should parse that first and use this as the fallback.
//
// Parameters:
//   - body: the upstream error body; a body larger than MaxUpstreamErrorBodyBytes
//     is not parsed, since it may be a truncated prefix.
//
// Returns:
//   - string: the trimmed provider message, or "" when none is recognized.
func UpstreamErrorBodyText(body []byte) string {
	if len(body) > MaxUpstreamErrorBodyBytes {
		return ""
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		// Not a JSON object: the caller describes the body generically.
		return ""
	}
	if nested, ok := root["error"]; ok {
		if text := upstreamJSONString(nested); text != "" {
			return text
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(nested, &object); err == nil {
			if text := upstreamJSONString(object["message"]); text != "" {
				return text
			}
		}
	}
	for _, key := range []string{"message", "detail"} {
		if text := upstreamJSONString(root[key]); text != "" {
			return text
		}
	}
	return ""
}

// UpstreamErrorBodyMessage returns a client-safe message for an upstream error
// body: the provider's recognized message (see UpstreamErrorBodyText), or else
// the generic UnrecognizedUpstreamErrorMessage, which never contains body bytes.
//
// Parameters:
//   - statusCode: the upstream HTTP status code.
//   - contentType: the provider's raw Content-Type header, or "" when unknown.
//   - body: the upstream error body as read; a body larger than
//     MaxUpstreamErrorBodyBytes is reported as oversized without being parsed.
//
// Returns:
//   - string: the message to place in Error.Message.
func UpstreamErrorBodyMessage(statusCode int, contentType string, body []byte) string {
	switch {
	case len(body) > MaxUpstreamErrorBodyBytes:
		return UnrecognizedUpstreamErrorMessage(statusCode, contentType, len(body), true)
	case len(bytes.TrimSpace(body)) == 0:
		return UnrecognizedUpstreamErrorMessage(statusCode, contentType, 0, false)
	}
	if text := UpstreamErrorBodyText(body); text != "" {
		return text
	}
	return UnrecognizedUpstreamErrorMessage(statusCode, contentType, len(body), false)
}

// upstreamJSONString decodes raw as a JSON string.
//
// Parameters:
//   - raw: one raw JSON value, possibly empty.
//
// Returns:
//   - string: the trimmed string, or "" when raw is not a JSON string.
func upstreamJSONString(raw json.RawMessage) string {
	if ErrorJSONKind(raw) != '"' {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		// A malformed string literal is absent like any other unusable kind.
		return ""
	}
	return strings.TrimSpace(text)
}

// sanitizeUpstreamMediaType reduces a provider Content-Type header to a media
// type that is safe to show to API callers.
//
// Parameters:
//   - contentType: the raw, provider-supplied Content-Type header.
//
// Returns:
//   - string: the lowercase "type/subtype" limited to RFC 6838 restricted-name
//     characters and maxUpstreamMediaTypeBytes, or "" when the header is absent,
//     too long, unparseable or contains other characters.
func sanitizeUpstreamMediaType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" || len(contentType) > maxUpstreamContentTypeBytes {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	// An invalid parameter still yields a valid media type; parameters are never echoed.
	if err != nil && !errors.Is(err, mime.ErrInvalidMediaParameter) {
		return ""
	}
	if len(mediaType) > maxUpstreamMediaTypeBytes {
		return ""
	}
	typeName, subtype, found := strings.Cut(mediaType, "/")
	if !found || typeName == "" || subtype == "" || strings.Contains(subtype, "/") {
		return ""
	}
	for i := 0; i < len(mediaType); i++ {
		switch c := mediaType[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '/':
		case strings.IndexByte("!#$&-^_.+", c) >= 0:
		default:
			return ""
		}
	}
	return mediaType
}
