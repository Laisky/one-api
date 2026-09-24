package controller

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// channelSelectionRequest captures a deliberate row selection and the list's applied search filter.
type channelSelectionRequest struct {
	Selection model.ListSelection `json:"selection"`
	Keyword   string              `json:"keyword,omitempty"`
}

// bindListSelectionRequest strictly decodes a bounded body and rejects empty or ambiguous JSON.
func bindListSelectionRequest(c *gin.Context, request any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		return errkind.InvalidRequestErr(errors.New("A valid selection request is required."))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errkind.InvalidRequestErr(errors.New("Only one selection request is allowed."))
	}
	return nil
}

// selectedChannelTargets decodes and resolves a selection; mutation calls require explicit confirmed UUIDs.
func selectedChannelTargets(c *gin.Context, explicitOnly bool) ([]model.ChannelModelResetTarget, error) {
	var request channelSelectionRequest
	if err := bindListSelectionRequest(c, &request); err != nil {
		return nil, errors.Wrap(err, "parse channel selection")
	}
	if explicitOnly && request.Selection.Mode != "ids" {
		return nil, errkind.InvalidRequestErr(errors.New("Resolve and confirm the selected UUIDs before changing channels."))
	}
	return model.SelectChannelTargets(gmw.Ctx(c), request.Selection, request.Keyword)
}

// ResolveChannelSelection returns a non-secret, bounded UUID snapshot for confirmation without changing channels.
func ResolveChannelSelection(c *gin.Context) {
	targets, err := selectedChannelTargets(c, false)
	if err != nil {
		respondListSelectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": targets})
}

// DeleteSelectedDisabledChannels applies deletion to confirmed UUIDs in c and returns
// per-channel results, distinguishing enabled or already-missing skips from failures.
func DeleteSelectedDisabledChannels(c *gin.Context) {
	lg := gmw.GetLogger(c)
	targets, err := selectedChannelTargets(c, true)
	if err != nil {
		respondListSelectionError(c, err)
		return
	}
	results := make([]gin.H, 0, len(targets))
	for _, target := range targets {
		deleted, err := model.DeleteSelectedDisabledChannel(gmw.Ctx(c), target.Id)
		outcome := gin.H{"uuid": target.UUID, "name": target.Name, "success": deleted, "skipped": !deleted && err == nil}
		if errors.Is(err, model.ErrSelectedChannelMissing) {
			outcome["skipped"] = true
			outcome["message"] = model.ErrSelectedChannelMissing.Error()
		} else if err != nil {
			lg.Error("selected channel deletion failed", zap.String("channel_uuid", target.UUID), zap.Error(err))
			outcome["message"] = "Failed to delete this channel."
		}
		results = append(results, outcome)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}

// respondListSelectionError emits an actionable client error or a sanitized operational failure for selection APIs.
func respondListSelectionError(c *gin.Context, err error) {
	if kind := errkind.Of(err); kind.IsClient() {
		helper.RespondErrorWithStatus(c, kind.HTTPStatus(), err)
		return
	}
	lg := gmw.GetLogger(c)
	lg.Error("list selection failed", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to process the selection. Refresh the list before retrying."})
}
