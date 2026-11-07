package controller

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// Strategy names reported by the count_tokens DEBUG log.
const (
	// claudeCountTokensStrategyUpstream answers with the native Anthropic counter.
	claudeCountTokensStrategyUpstream = "anthropic_upstream"
	// claudeCountTokensStrategyLocal answers with the gateway's local estimate.
	claudeCountTokensStrategyLocal = "local_estimate"
	// claudeCountTokensStrategyFallback answers with the local estimate because
	// the native Anthropic counter was unavailable.
	claudeCountTokensStrategyFallback = "local_estimate_after_upstream_failure"
)

// claudeCountTokensFailure is a client-facing count_tokens failure.
type claudeCountTokensFailure struct {
	// status is the HTTP status written downstream.
	status int
	// err is the underlying cause; its message is shown to the client.
	err error
}

// RelayClaudeCountTokens serves POST /v1/messages/count_tokens, Anthropic's
// free token-counting endpoint used by Claude Code for /context.
//
// The route runs the same token authentication, model permission, rate limit
// and channel distribution middlewares as /v1/messages. The handler then
// answers {"input_tokens": N}: a native Anthropic channel is asked through its
// own /v1/messages/count_tokens endpoint, and every other channel (or a native
// counter that is unavailable) is answered with the gateway's local estimate.
//
// Billing guarantee: this handler never reserves, consumes, refunds or settles
// quota, writes no consume log or request-cost row, and never calls a
// generation endpoint. Errors use the Anthropic error envelope.
//
// Parameters:
//   - c: the request context prepared by the relay middlewares.
func RelayClaudeCountTokens(c *gin.Context) {
	lg := gmw.GetLogger(c)
	request, raw, failure := getAndValidateClaudeCountTokensRequest(c)
	if failure != nil {
		writeClaudeCountTokensFailure(c, lg, failure)
		return
	}

	meta := metalib.GetByContext(c)
	meta.IsStream = false
	meta.OriginModelName = request.Model
	request.Model = meta.ActualModelName
	request.CompatibilityModel = anthropic.CompatibilityModel(c, request.Model)
	metalib.Set2Context(c, meta)

	strategy := claudeCountTokensStrategyLocal
	if meta.ChannelType == channeltype.Anthropic {
		if status, written := forwardClaudeCountTokens(c, meta, request, raw); written {
			lg.Debug("served claude count_tokens",
				zap.String("strategy", claudeCountTokensStrategyUpstream),
				zap.Int("status_code", status),
				zap.String("origin_model", meta.OriginModelName),
				zap.String("model", meta.ActualModelName),
				zap.Int("channel_type", meta.ChannelType),
			)
			return
		}
		strategy = claudeCountTokensStrategyFallback
	}

	tokens, failure := estimateClaudeCountTokens(c, request, raw)
	if failure != nil {
		writeClaudeCountTokensFailure(c, lg, failure)
		return
	}
	c.JSON(http.StatusOK, gin.H{"input_tokens": tokens})
	lg.Debug("served claude count_tokens",
		zap.String("strategy", strategy),
		zap.Int("input_tokens", tokens),
		zap.String("origin_model", meta.OriginModelName),
		zap.String("model", meta.ActualModelName),
		zap.Int("channel_type", meta.ChannelType),
	)
}

// getAndValidateClaudeCountTokensRequest decodes and validates a count_tokens
// body. It is the Messages body without max_tokens, so it shares the Messages
// content validation but does not require max_tokens.
//
// Parameters:
//   - c: the request context whose body was buffered by the route middlewares.
//
// Return values:
//   - *ClaudeMessagesRequest: the decoded request.
//   - []byte: the raw body bytes.
//   - *claudeCountTokensFailure: a 400/413 failure, or nil.
func getAndValidateClaudeCountTokensRequest(c *gin.Context) (*ClaudeMessagesRequest, []byte, *claudeCountTokensFailure) {
	raw, err := common.GetRequestBody(c)
	if err != nil {
		return nil, nil, claudeCountTokensBodyReadFailure(err)
	}
	request := &ClaudeMessagesRequest{}
	if err := common.UnmarshalBodyReusable(c, request); err != nil {
		return nil, nil, &claudeCountTokensFailure{
			status: http.StatusBadRequest,
			err:    errors.Wrap(err, "invalid count_tokens request body"),
		}
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, nil, &claudeCountTokensFailure{status: http.StatusBadRequest, err: errors.New("model is required")}
	}
	if err := validateClaudeMessagesContent(request); err != nil {
		return nil, nil, &claudeCountTokensFailure{status: http.StatusBadRequest, err: errors.WithStack(err)}
	}
	return request, raw, nil
}

// claudeCountTokensBodyReadFailure classifies a body read error as 413 when a
// byte cap rejected the body and 400 otherwise.
//
// Parameters:
//   - err: the read error.
//
// Return values:
//   - *claudeCountTokensFailure: the client-facing failure.
func claudeCountTokensBodyReadFailure(err error) *claudeCountTokensFailure {
	var limitError *http.MaxBytesError
	if errors.As(err, &limitError) || errors.Is(err, middleware.ErrRequestBodyTooLarge) {
		return &claudeCountTokensFailure{
			status: http.StatusRequestEntityTooLarge,
			err:    errors.Wrap(err, "request body exceeds the count_tokens size limit"),
		}
	}
	return &claudeCountTokensFailure{status: http.StatusBadRequest, err: errors.Wrap(err, "read count_tokens request body")}
}

// writeClaudeCountTokensFailure logs a count_tokens failure once (WARN for a
// client error, ERROR for a server fault) and writes it in Anthropic format.
//
// Parameters:
//   - c: the request context.
//   - lg: the request-scoped logger of the caller.
//   - failure: the failure to report.
func writeClaudeCountTokensFailure(c *gin.Context, lg glog.Logger, failure *claudeCountTokensFailure) {
	if failure.status >= http.StatusInternalServerError {
		lg.Error("claude count_tokens failed", zap.Int("status_code", failure.status), zap.Error(failure.err))
	} else {
		lg.Warn("rejected claude count_tokens request", zap.Int("status_code", failure.status), zap.String("error", failure.err.Error()))
	}
	if c.Writer.Written() {
		return
	}
	middleware.AbortWithClaudeError(c, failure.status, failure.err.Error())
}
