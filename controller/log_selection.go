package controller

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// logSelectionRequest contains list filters and a selection but never a client-supplied authorization scope.
type logSelectionRequest struct {
	Selection      model.ListSelection `json:"selection"`
	Keyword        string              `json:"keyword,omitempty"`
	Type           int                 `json:"type,omitempty"`
	StartTimestamp int64               `json:"start_timestamp,omitempty"`
	EndTimestamp   int64               `json:"end_timestamp,omitempty"`
	ModelName      string              `json:"model_name,omitempty"`
	TokenName      string              `json:"token_name,omitempty"`
	Username       string              `json:"username,omitempty"`
	Channel        string              `json:"channel,omitempty"`
	Sort           string              `json:"sort,omitempty"`
	Order          string              `json:"order,omitempty"`
}

// ResolveLogSelection resolves the caller's selected rows for export or deletion confirmation using boundary DTOs.
func ResolveLogSelection(c *gin.Context) {
	id, role := c.GetInt(ctxkey.Id), c.GetInt(ctxkey.Role)
	if id <= 0 {
		helper.RespondErrorWithStatus(c, http.StatusUnauthorized, errors.New("Authentication is required."))
		return
	}
	var request logSelectionRequest
	if err := bindListSelectionRequest(c, &request); err != nil {
		respondListSelectionError(c, err)
		return
	}
	scope := model.LogListScope{Kind: model.LogListScopeSelf, SubjectUserID: id, PrincipalUserID: id, Role: role}
	filter := model.LogListFilter{LogType: request.Type, StartTimestamp: request.StartTimestamp, EndTimestamp: request.EndTimestamp,
		ModelName: request.ModelName, TokenName: request.TokenName}
	if role >= model.RoleAdminUser {
		scope.Kind = model.LogListScopeAll
		scope.SubjectUserID = 0
		filter.Username = request.Username
		if request.Keyword == "" {
			channel, err := resolveOptionalChannelRef(request.Channel)
			if err != nil {
				respondListSelectionError(c, err)
				return
			}
			filter.ChannelID = channel
		}
	}
	logs, err := model.SelectLogs(gmw.Ctx(c), scope, filter, request.Keyword, request.Selection, request.Sort, request.Order)
	if err != nil {
		respondListSelectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": model.LogsToResponses(logs)})
}

// DeleteSelectedLogs requires administrator authorization again before deleting a confirmed explicit UUID snapshot.
func DeleteSelectedLogs(c *gin.Context) {
	if c.GetInt(ctxkey.Role) < model.RoleAdminUser {
		helper.RespondErrorWithStatus(c, http.StatusForbidden, errors.New("Administrative privileges are required."))
		return
	}
	var request struct {
		Selection model.ListSelection `json:"selection"`
	}
	if err := bindListSelectionRequest(c, &request); err != nil {
		respondListSelectionError(c, err)
		return
	}
	deleted, err := model.DeleteSelectedLogs(gmw.Ctx(c), request.Selection)
	if err != nil {
		respondListSelectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": deleted}})
}
