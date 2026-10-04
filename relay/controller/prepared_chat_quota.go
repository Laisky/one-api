package controller

import (
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/replicate"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

// preparedChatQuotaRequest returns a private quota view using the converted provider's actual output limit.
// Providers that preserve the canonical Chat request keep their existing quota semantics.
func preparedChatQuotaRequest(c *gin.Context, request *relaymodel.GeneralOpenAIRequest) *relaymodel.GeneralOpenAIRequest {
	converted, _ := c.Get(ctxkey.ConvertedRequest)
	var limit int
	switch payload := converted.(type) {
	case *anthropic.Request:
		limit = payload.MaxTokens
	case replicate.ReplicateChatRequest:
		limit = payload.Input.MaxTokens
	default:
		return request
	}
	quote := *request
	quote.MaxTokens = limit
	quote.MaxCompletionTokens = nil
	return &quote
}
