package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// claudeCountTokensMaxResponseBytes bounds the upstream count_tokens reply the
// gateway buffers. A real reply is a few dozen bytes.
const claudeCountTokensMaxResponseBytes = 1 << 20

// claudeCountTokensUpstream reuses the shared Anthropic request pipeline
// (channel credentials, anthropic-version / anthropic-beta handling, channel
// custom headers, transport observation) but targets the count_tokens URL.
// It is only ever passed to adaptor.DoRequestHelper; it is never registered as
// a channel adaptor and has no generation or billing role.
type claudeCountTokensUpstream struct {
	*anthropic.Adaptor
	// url is the resolved upstream count_tokens URL.
	url string
}

// GetRequestURL returns the resolved count_tokens URL instead of /v1/messages.
//
// Parameters:
//   - _: the request metadata (unused; the URL was resolved up front).
//
// Return values:
//   - string: the count_tokens URL.
//   - error: always nil.
func (a *claudeCountTokensUpstream) GetRequestURL(_ *metalib.Meta) (string, error) {
	return a.url, nil
}

// SetupRequestHeader applies the native Anthropic headers, then pins the JSON
// content negotiation the count_tokens endpoint speaks.
//
// Parameters:
//   - c: the request context.
//   - req: the outbound upstream request.
//   - meta: the request metadata carrying the channel credential.
//
// Return values:
//   - error: a header setup error.
func (a *claudeCountTokensUpstream) SetupRequestHeader(c *gin.Context, req *http.Request, meta *metalib.Meta) error {
	if err := a.Adaptor.SetupRequestHeader(c, req, meta); err != nil {
		return errors.Wrap(err, "setup Anthropic count_tokens headers")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

// claudeCountTokensURL resolves the upstream count_tokens URL of a native
// Anthropic channel. An administrator endpoint override for Claude Messages is
// honored only when it ends in ".../messages", so count_tokens can never be
// sent to a URL that serves generations.
//
// Parameters:
//   - meta: the request metadata of the selected channel.
//
// Return values:
//   - string: the count_tokens URL.
//   - bool: false when no safe URL can be derived.
func claudeCountTokensURL(meta *metalib.Meta) (string, bool) {
	if override := meta.UpstreamEndpointURLOverride(); override != "" {
		parsed, err := url.Parse(override)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", false
		}
		trimmed := strings.TrimRight(parsed.Path, "/")
		if !strings.HasSuffix(trimmed, "/messages") {
			return "", false
		}
		parsed.Path = trimmed + "/count_tokens"
		parsed.RawPath = ""
		return parsed.String(), true
	}
	base := strings.TrimRight(strings.TrimSpace(meta.BaseURL), "/")
	if base == "" {
		return "", false
	}
	return base + "/v1/messages/count_tokens", true
}

// claudeCountTokensUpstreamRejectsRequest reports whether an upstream status
// describes the request itself (so it is returned to the client) rather than
// the channel (credentials, rate limits, a missing endpoint on an
// Anthropic-compatible proxy, or a server fault), which falls back to the local
// estimate.
//
// Parameters:
//   - status: the upstream HTTP status.
//
// Return values:
//   - bool: true for 400, 413 and 422.
func claudeCountTokensUpstreamRejectsRequest(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

// forwardClaudeCountTokens asks the selected native Anthropic channel's
// count_tokens endpoint and writes its answer downstream. Success and request
// errors (400/413/422) are written; every other outcome (no derivable URL,
// body preparation failure, transport error, channel-level 4xx, 5xx, or a
// malformed reply) is logged once and reported as not written so the caller
// answers with the local estimate. It never calls a generation endpoint.
//
// Parameters:
//   - c: the request context.
//   - meta: the request metadata of the selected channel.
//   - request: the decoded request with the mapped model.
//   - raw: the raw request body.
//
// Return values:
//   - int: the upstream status when written, otherwise 0.
//   - bool: true when a response was written downstream.
func forwardClaudeCountTokens(c *gin.Context, meta *metalib.Meta, request *ClaudeMessagesRequest, raw []byte) (int, bool) {
	lg := gmw.GetLogger(c)
	upstreamURL, ok := claudeCountTokensURL(meta)
	if !ok {
		lg.Debug("claude count_tokens has no safe upstream URL for this channel; using local estimate")
		return 0, false
	}

	// Send the same prompt the generation passthrough would send: mapped
	// model, normalized controls and unsigned thinking removed.
	sanitizeClaudeMessagesRequest(request)
	body, _, err := rewriteAndSanitizeClaudeRequestBody(raw, request)
	if err != nil {
		lg.Warn("claude count_tokens body preparation failed; using local estimate", zap.String("error", err.Error()))
		return 0, false
	}
	prepared, err := anthropic.PrepareRequestBody(c, meta.ActualModelName, bytes.NewReader(body))
	if err != nil {
		lg.Warn("claude count_tokens body normalization failed; using local estimate", zap.String("error", err.Error()))
		return 0, false
	}
	c.Set(ctxkey.ClaudeToolSearchEnabled, false)
	c.Set(ctxkey.ClaudeThinkingBindingControlsEnabled, request.Thinking != nil && request.Thinking.BlockBinding != nil)

	upstreamMeta := *meta
	upstreamMeta.IsStream = false
	// The override (if any) is already resolved into upstreamURL; clearing it on
	// the copy stops the shared helper from replacing the URL with the Messages one.
	upstreamMeta.Config.EndpointURLs = nil
	provider := &claudeCountTokensUpstream{Adaptor: &anthropic.Adaptor{}, url: upstreamURL}
	provider.Init(&upstreamMeta)
	resp, err := adaptor.DoRequestHelper(provider, c, &upstreamMeta, prepared)
	if err != nil {
		lg.Warn("claude count_tokens upstream request failed; using local estimate", zap.String("error", err.Error()))
		return 0, false
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			lg.Debug("close claude count_tokens upstream body", zap.String("error", closeErr.Error()))
		}
	}()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, claudeCountTokensMaxResponseBytes+1))
	if err == nil && len(payload) > claudeCountTokensMaxResponseBytes {
		err = errors.New("upstream count_tokens reply exceeds the size limit")
	}
	if err != nil {
		lg.Warn("claude count_tokens upstream reply unreadable; using local estimate",
			zap.Int("upstream_status", resp.StatusCode), zap.String("error", err.Error()))
		return 0, false
	}
	return writeClaudeCountTokensUpstreamReply(c, resp, payload)
}

// writeClaudeCountTokensUpstreamReply writes a successful count_tokens reply
// or a request-level upstream error downstream with the Anthropic correlation
// and rate-limit headers, or logs why the reply is not usable.
//
// Parameters:
//   - c: the request context.
//   - resp: the upstream response whose body was read into payload.
//   - payload: the upstream body bytes.
//
// Return values:
//   - int: the status written, or 0.
//   - bool: true when a response was written downstream.
func writeClaudeCountTokensUpstreamReply(c *gin.Context, resp *http.Response, payload []byte) (int, bool) {
	lg := gmw.GetLogger(c)
	status := resp.StatusCode
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		var reply struct {
			InputTokens *int64 `json:"input_tokens"`
		}
		if err := json.Unmarshal(payload, &reply); err != nil || reply.InputTokens == nil || *reply.InputTokens < 0 {
			lg.Warn("claude count_tokens upstream reply has no valid input_tokens; using local estimate",
				zap.Int("upstream_status", status), zap.Int("reply_bytes", len(payload)))
			return 0, false
		}
		anthropic.CopyClaudeResponseHeaders(c.Writer.Header(), resp.Header)
		c.Data(status, "application/json", payload)
		return status, true
	case claudeCountTokensUpstreamRejectsRequest(status):
		var envelope struct {
			Type  string `json:"type"`
			Error *struct {
				Type    string  `json:"type"`
				Message *string `json:"message"`
			} `json:"error"`
		}
		native := json.Unmarshal(payload, &envelope) == nil && envelope.Type == "error" &&
			envelope.Error != nil && envelope.Error.Type != "" && envelope.Error.Message != nil
		errorType := ""
		if native {
			errorType = envelope.Error.Type
		}
		lg.Warn("upstream rejected claude count_tokens request",
			zap.Int("upstream_status", status), zap.String("upstream_error_type", errorType))
		anthropic.CopyClaudeResponseHeaders(c.Writer.Header(), resp.Header)
		if native {
			c.Data(status, "application/json", payload)
		} else {
			middleware.AbortWithClaudeError(c, status, "the upstream provider rejected the count_tokens request")
		}
		return status, true
	default:
		fields := []zap.Field{zap.Int("upstream_status", status)}
		if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
			// Anthropic-compatible proxies often lack count_tokens; expected and noisy.
			lg.Debug("claude count_tokens unsupported by upstream; using local estimate", fields...)
		} else {
			lg.Warn("claude count_tokens upstream unavailable; using local estimate", fields...)
		}
		return 0, false
	}
}
