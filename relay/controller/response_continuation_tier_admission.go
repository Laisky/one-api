package controller

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

// errResponseContinuationStoreUnavailable classifies operational parent reads
// through quote wrappers so transient storage failures remain retryable.
var errResponseContinuationStoreUnavailable = errors.New("owned Responses parent storage is unavailable")

// errResponseContinuationUnpriceable rejects opaque inherited context rather
// than authorizing paid provider work with an incremental-only allowance.
var errResponseContinuationUnpriceable = errors.New("cannot safely price inherited Responses context; start a new request with complete input without previous_response_id")

// responseContinuationTierPrompt augments a tier allowance with known inherited
// provider context. Parameters: c identifies the owner-resolved gateway parent,
// meta supplies the current provider/model, request holds the prepared upstream
// handle, and incremental is the actual current-body counter. Returns: checked
// inherited-plus-incremental input or a wrapped lookup/arithmetic error.
// Active paid tiers fail closed when the saved parent cannot establish a budget;
// callers bypass this lookup for non-tier contracts and authoritative free groups.
func responseContinuationTierPrompt(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest, incremental int) (int, error) {
	if request == nil || request.PreviousResponseId == nil {
		return incremental, nil
	}
	if c == nil || meta == nil || *request.PreviousResponseId == "" ||
		!state.Enabled() || state.Store() == nil {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "continuation metadata or state is unavailable")
	}
	parentID := c.GetString(ctxNativeGatewayParent)
	owner := stateOwnerFromMeta(meta)
	if parentID == "" || !owner.Valid() {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "owned continuation parent is unavailable")
	}
	ctx, cancel := context.WithTimeout(gmw.Ctx(c), 5*time.Second)
	defer cancel()
	parent, err := state.Store().GetResponse(ctx, owner, parentID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return 0, errors.Wrap(errResponseContinuationUnpriceable, "owned continuation parent was not found")
		}
		return 0, errors.Wrap(errors.Join(errResponseContinuationStoreUnavailable, err), "load owned Responses parent usage")
	}
	if parent == nil || parent.Owner != owner || parent.Binding == nil {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "owned continuation binding is unavailable")
	}
	switch parent.Status {
	case state.StatusCompleted, state.StatusIncomplete, state.StatusFailed, state.StatusCancelled:
	default:
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "continuation parent is not terminal")
	}
	binding := parent.Binding
	if binding.ChannelID != meta.ChannelId || binding.APIType != meta.APIType ||
		binding.ActualModel != request.Model || binding.ActualModel != meta.ActualModelName ||
		binding.UpstreamResponseID == "" || binding.UpstreamResponseID != *request.PreviousResponseId {
		return 0, errors.Wrap(errResponseContinuationUnpriceable, "continuation parent pricing identity does not match")
	}
	// Keep HTTP and native WebSocket continuations on the same explicit
	// inherited-usage validation contract.
	inherited, err := responseTierInheritedTokens(parent.Usage)
	if err != nil {
		return 0, err
	}
	if incremental < 0 || inherited > int(^uint(0)>>1)-incremental {
		return 0, errors.New("owned Responses continuation token budget overflows")
	}
	return inherited + incremental, nil
}
