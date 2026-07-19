package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// conversationHandler adapts a relay-controller conversation helper (which
// returns a typed error) into a gin handler. Conversation CRUD performs no
// upstream call and never enters channel distribution (proposal row V11).
func conversationHandler(fn func(*gin.Context) *relaymodel.ErrorWithStatusCode) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bizErr := fn(c); bizErr != nil {
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
