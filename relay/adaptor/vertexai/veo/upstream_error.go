package veo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

const (
	// maxErrorBodyBytes bounds every provider error body one-api reads.
	maxErrorBodyBytes = 1 << 20
	// maxCreateBodyBytes bounds the predictLongRunning response, which only
	// names the long-running operation.
	maxCreateBodyBytes = 1 << 20
	// maxPollBodyBytes bounds a fetchPredictOperation response. A completed
	// operation inlines base64 video bytes when the request set no storageUri,
	// so it is far larger than an ordinary JSON envelope.
	maxPollBodyBytes = 256 << 20
	// providerMessageLimit bounds a provider message copied to the client.
	providerMessageLimit = 1024
)

// providerStatusPattern accepts only google.rpc status names such as
// RESOURCE_EXHAUSTED, so a provider cannot inject arbitrary text through them.
var providerStatusPattern = regexp.MustCompile(`^[A-Z][A-Z_]{0,63}$`)

// readBoundedBody reads at most limit bytes from body.
// It accepts the body reader and the byte limit, and returns the bytes read
// (truncated to limit), whether the body exceeded limit, and a wrapped read
// error. A nil body yields no bytes and no error.
func readBoundedBody(body io.Reader, limit int64) ([]byte, bool, error) {
	if body == nil {
		return nil, false, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return data, false, errors.Wrap(err, "read upstream response body")
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

// googleErrorEnvelope is the Google API error body
// {"error":{"code":..,"message":..,"status":..}}.
type googleErrorEnvelope struct {
	Error *struct {
		Message json.RawMessage `json:"message"`
		Status  json.RawMessage `json:"status"`
	} `json:"error"`
}

// upstreamErrorMessage derives a client-safe message from a provider error
// body without echoing raw body bytes.
// It accepts the (possibly truncated or empty) body and the provider HTTP
// status, and returns the sanitized Google error envelope message when present,
// else a generic "upstream returned HTTP <status>" message that names the
// google.rpc status when it is identifier-shaped.
func upstreamErrorMessage(body []byte, statusCode int) string {
	message, status := parseGoogleError(body)
	if message != "" {
		return message
	}
	generic := fmt.Sprintf("upstream returned HTTP %d", statusCode)
	if status != "" {
		generic += " (" + status + ")"
	}
	return generic
}

// parseGoogleError extracts the message and status of a Google error envelope.
// It accepts the body, which may be a single envelope or an array whose first
// element is one, and returns the sanitized message and the identifier-shaped
// status; either is empty when absent or unsafe.
func parseGoogleError(body []byte) (string, string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", ""
	}
	var envelope googleErrorEnvelope
	if trimmed[0] == '[' {
		var list []googleErrorEnvelope
		if json.Unmarshal(trimmed, &list) != nil || len(list) == 0 {
			return "", ""
		}
		envelope = list[0]
	} else if json.Unmarshal(trimmed, &envelope) != nil {
		return "", ""
	}
	if envelope.Error == nil {
		return "", ""
	}
	status := jsonString(envelope.Error.Status)
	if !providerStatusPattern.MatchString(status) {
		status = ""
	}
	return sanitizeProviderMessage(jsonString(envelope.Error.Message)), status
}

// jsonString decodes raw as a JSON string.
// It accepts a raw JSON value and returns the decoded string, or "" when raw
// is absent or not a JSON string.
func jsonString(raw json.RawMessage) string {
	var text string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil {
		return ""
	}
	return text
}

// sanitizeProviderMessage makes a provider message safe to show a client.
// It accepts the provider text and returns it trimmed, with control characters
// replaced by spaces and bounded to providerMessageLimit bytes on a rune
// boundary, or "" when the text is empty or not valid UTF-8.
func sanitizeProviderMessage(text string) string {
	if !utf8.ValidString(text) {
		return ""
	}
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if len(text) <= providerMessageLimit {
		return text
	}
	limit := providerMessageLimit
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return strings.TrimSpace(text[:limit])
}
