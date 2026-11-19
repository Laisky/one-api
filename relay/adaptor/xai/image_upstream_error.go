package xai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// imageUpstreamErrorCode is the client-visible code of an xAI image refusal
// that carries no code of its own; it mirrors the video path's
// "upstream_video_error".
const imageUpstreamErrorCode = "upstream_image_error"

// imageErrorCode returns the provider's error code from an xAI error body. It
// accepts both xAI shapes: the flat {"code":"AccountQuotaExceeded","error":"..."}
// body and the OpenAI-style {"error":{"code":"..."}} body. Only JSON string
// codes count; anything else is absent so that no raw provider data reaches the
// client.
//
// Parameters:
//   - body: the bounded error body; an over-limit body is not parsed.
//
// Returns:
//   - string: the trimmed provider code, or "".
func imageErrorCode(body []byte) string {
	if len(body) > model.MaxUpstreamErrorBodyBytes {
		return ""
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	if code := imageJSONString(root["code"]); code != "" {
		return code
	}
	var nested map[string]json.RawMessage
	if len(root["error"]) > 0 && json.Unmarshal(root["error"], &nested) == nil {
		return imageJSONString(nested["code"])
	}
	return ""
}

// imageJSONString returns raw as a trimmed string when it is a JSON string.
//
// Parameters:
//   - raw: a raw JSON member, possibly empty.
//
// Returns:
//   - string: the decoded, trimmed string, or "" for any other JSON kind.
func imageJSONString(raw json.RawMessage) string {
	if model.ErrorJSONKind(raw) != '"' {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

// imageStatusError converts a non-2xx xAI image answer (for example 429
// AccountQuotaExceeded or 503 overloaded) into a provider-marked relay error
// that keeps the provider's HTTP status. The client sees
// "upstream returned HTTP N: <message>" when the body carries a message that
// model.UpstreamErrorBodyText recognizes, otherwise the shared generic
// description, and the provider code (default "upstream_image_error"); the raw
// body is never echoed. It reads, but does not close, resp.Body; the caller
// owns closing it.
//
// Parameters:
//   - resp: the non-2xx xAI response.
//
// Returns:
//   - *model.ErrorWithStatusCode: the error, with FromUpstream set and StatusCode
//     equal to resp.StatusCode.
func imageStatusError(resp *http.Response) *model.ErrorWithStatusCode {
	body, readErr := model.ReadBoundedUpstreamBody(resp.Body)
	if readErr != nil {
		result := model.NewUpstreamError(resp.StatusCode, model.UnreadableUpstreamErrorMessage(resp.StatusCode),
			model.ErrorTypeUpstream, imageUpstreamErrorCode)
		// Keep the transport failure for diagnostics; the provider status still
		// identifies the refusal, so the error stays provider-marked.
		result.RawError = errors.Wrapf(readErr, "read xAI HTTP %d image error body", resp.StatusCode)
		return result
	}

	message := model.UpstreamErrorBodyMessage(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	if text := model.UpstreamErrorBodyText(body); text != "" {
		message = fmt.Sprintf("upstream returned HTTP %d: %s", resp.StatusCode, text)
	}
	code := imageUpstreamErrorCode
	if providerCode := imageErrorCode(body); providerCode != "" {
		code = providerCode
	}
	return model.NewUpstreamError(resp.StatusCode, message, model.ErrorTypeUpstream, code)
}
