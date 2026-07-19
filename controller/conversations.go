package controller

import (
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// logRelayStateBizError logs a relay/state biz error by severity (ST-023,
// following the repository's log-level-by-status convention): typed 4xx
// client/compatibility errors log at WARN without a stack trace; 5xx server-side
// failures log at ERROR with the underlying error for alerting. Only the stable
// machine code and status are logged — never prompts, payloads, or state content.
func logRelayStateBizError(c *gin.Context, scope string, bizErr *relaymodel.ErrorWithStatusCode) {
	if bizErr == nil {
		return
	}
	fields := []zap.Field{
		zap.String("scope", scope),
		zap.Int("status", bizErr.StatusCode),
		zap.Any("code", bizErr.Error.Code),
	}
	lg := gmw.GetLogger(c)
	if bizErr.StatusCode >= http.StatusInternalServerError {
		if bizErr.RawError != nil {
			fields = append(fields, zap.Error(bizErr.RawError))
		}
		lg.Error("relay request failed", fields...)
		return
	}
	lg.Warn("relay request rejected", fields...)
}

// conversationHandler adapts a relay-controller conversation helper (which
// returns a typed error) into a gin handler. Conversation CRUD performs no
// upstream call and never enters channel distribution (proposal row V11).
func conversationHandler(fn func(*gin.Context) *relaymodel.ErrorWithStatusCode) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bizErr := fn(c); bizErr != nil {
			logRelayStateBizError(c, "conversations", bizErr)
			requestId := c.GetString(helper.RequestIdKey)
			bizErr.Error.Message = helper.MessageWithRequestId(bizErr.Error.Message, requestId)
			c.JSON(bizErr.StatusCode, gin.H{"error": bizErr.Error})
		}
	}
}

// RelayConversationCreate handles POST /v1/conversations.
func RelayConversationCreate(c *gin.Context) {
	conversationHandler(rcontroller.ConversationCreateHelper)(c)
}

// RelayConversationGet handles GET /v1/conversations/{id}.
func RelayConversationGet(c *gin.Context) {
	conversationHandler(rcontroller.ConversationGetHelper)(c)
}

// RelayConversationUpdate handles POST /v1/conversations/{id}.
func RelayConversationUpdate(c *gin.Context) {
	conversationHandler(rcontroller.ConversationUpdateHelper)(c)
}

// RelayConversationDelete handles DELETE /v1/conversations/{id}.
func RelayConversationDelete(c *gin.Context) {
	conversationHandler(rcontroller.ConversationDeleteHelper)(c)
}

// RelayConversationItemsCreate handles POST /v1/conversations/{id}/items.
func RelayConversationItemsCreate(c *gin.Context) {
	conversationHandler(rcontroller.ConversationItemsCreateHelper)(c)
}

// RelayConversationItemsList handles GET /v1/conversations/{id}/items.
func RelayConversationItemsList(c *gin.Context) {
	conversationHandler(rcontroller.ConversationItemsListHelper)(c)
}

// RelayConversationItemGet handles GET /v1/conversations/{id}/items/{item_id}.
func RelayConversationItemGet(c *gin.Context) {
	conversationHandler(rcontroller.ConversationItemGetHelper)(c)
}

// RelayConversationItemDelete handles DELETE /v1/conversations/{id}/items/{item_id}.
func RelayConversationItemDelete(c *gin.Context) {
	conversationHandler(rcontroller.ConversationItemDeleteHelper)(c)
}
