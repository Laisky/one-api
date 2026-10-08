package controller

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

// errResponseContinuationStoreUnavailable classifies operational parent reads
// through quote wrappers without changing legacy not-found compatibility.
var errResponseContinuationStoreUnavailable = errors.New("owned Responses parent storage is unavailable")

// responseContinuationTierPrompt augments a tier allowance with known inherited
// provider context. Parameters: c identifies the owner-resolved gateway parent,
// meta supplies the current provider/model, request holds the prepared upstream
// handle, and incremental is the actual current-body counter. Returns: checked
// inherited-plus-incremental input or a wrapped lookup/arithmetic error.
// Missing, malformed, negative or incompatible parent receipts retain legacy
// incremental quoting pending an explicit compatibility policy; those cases
// remain an unresolved admission gap and are not evidence of full protection.
func responseContinuationTierPrompt(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest, incremental int) (int, error) {
	if c == nil || meta == nil || request == nil || request.PreviousResponseId == nil ||
		!state.Enabled() || state.Store() == nil {
		return incremental, nil
	}
	parentID := c.GetString(ctxNativeGatewayParent)
	owner := stateOwnerFromMeta(meta)
	if parentID == "" || !owner.Valid() {
		return incremental, nil
	}
	ctx, cancel := context.WithTimeout(gmw.Ctx(c), 5*time.Second)
	defer cancel()
	parent, err := state.Store().GetResponse(ctx, owner, parentID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return incremental, nil
		}
		return 0, errors.Wrap(errors.Join(errResponseContinuationStoreUnavailable, err), "load owned Responses parent usage")
	}
	if parent == nil || parent.Owner != owner || parent.Binding == nil {
		return incremental, nil
	}
	switch parent.Status {
	case state.StatusCompleted, state.StatusIncomplete, state.StatusFailed, state.StatusCancelled:
	default:
		return incremental, nil
	}
	binding := parent.Binding
	if binding.ChannelID != meta.ChannelId || binding.APIType != meta.APIType ||
		binding.ActualModel != request.Model || binding.ActualModel != meta.ActualModelName ||
		binding.UpstreamResponseID == "" || binding.UpstreamResponseID != *request.PreviousResponseId {
		return incremental, nil
	}
	// Pointers distinguish an authoritative zero from an absent counter. Native
	// state commits preserve the provider receipt without synthetic billing usage.
	var usage struct {
		InputTokens  *int `json:"input_tokens"`
		OutputTokens *int `json:"output_tokens"`
	}
	if len(parent.Usage) == 0 {
		return incremental, nil
	}
	if err := json.Unmarshal(parent.Usage, &usage); err != nil {
		lg := gmw.GetLogger(c)
		lg.Debug("retaining legacy Responses continuation quote for invalid saved usage",
			zap.Error(errors.Wrap(err, "decode owned Responses parent usage")))
		return incremental, nil
	}
	if usage.InputTokens == nil || usage.OutputTokens == nil ||
		*usage.InputTokens < 0 || *usage.OutputTokens < 0 {
		return incremental, nil
	}
	if incremental < 0 || *usage.InputTokens > math.MaxInt-*usage.OutputTokens {
		return 0, errors.WithStack(errors.New("owned Responses parent token budget is invalid or overflows"))
	}
	inherited := *usage.InputTokens + *usage.OutputTokens
	if incremental > math.MaxInt-inherited {
		return 0, errors.WithStack(errors.New("owned Responses continuation token budget overflows"))
	}
	return inherited + incremental, nil
}
