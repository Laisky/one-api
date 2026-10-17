package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// prepareResponseFallbackInput validates caller input before snapshotting or
// hydrating it, then bounds the resulting effective turn before normalization.
// ctx carries cancellation, c stores the pending state commit, meta supplies
// ownership/routing, and request supplies the caller's Responses input. It
// returns the hydrated request or the existing state/admission error without
// dispatching upstream work. The limits do not depend on state persistence.
func prepareResponseFallbackInput(ctx context.Context, c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest) (*openai.ResponseAPIRequest, *relaymodel.ErrorWithStatusCode) {
	if request == nil {
		return nil, openai.ErrorWrapper(errors.New("response api request is nil"), "invalid_response_api_request", http.StatusBadRequest)
	}
	if unsupported := responseToolSearchFallbackError(request); unsupported != nil {
		return nil, unsupported
	}
	// Chat/Claude conversion cannot preserve namespace-qualified call identities.
	// Reject before state hydration or quota reservation rather than dropping tools.
	for _, tool := range request.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Type), "namespace") {
			return nil, openai.ErrorWrapper(errors.New("tool namespaces require native Responses support"), "tool_namespace_not_supported", http.StatusBadRequest)
		}
	}
	if err := openai.ValidateResponseAPIFallbackInput(request.Input); err != nil {
		return nil, openai.ErrorWrapper(err, "response_fallback_input_too_large", http.StatusRequestEntityTooLarge)
	}

	if err := ctx.Err(); err != nil {
		return nil, openai.ErrorWrapper(err, "response_fallback_cancelled", http.StatusRequestTimeout)
	}

	// Preserve the original incremental snapshot and hydration order: validation
	// adds an admission boundary, not a second conversation or ownership model.
	capturePendingStateCommit(c, meta, request)
	hydrated, stateErr := hydrateResponseAPIRequestForFallback(ctx, meta, request, responseFallbackTarget(meta))
	if stateErr != nil {
		return nil, stateErr
	}
	if hydrated == nil {
		return nil, openai.ErrorWrapper(errors.New("response fallback hydration returned no request"), "response_fallback_hydration_failed", http.StatusInternalServerError)
	}
	if unsupported := responseToolSearchFallbackError(hydrated); unsupported != nil {
		return nil, unsupported
	}
	if err := openai.ValidateResponseAPIFallbackInput(hydrated.Input); err != nil {
		return nil, openai.ErrorWrapper(err, "response_fallback_input_too_large", http.StatusRequestEntityTooLarge)
	}
	return hydrated, nil
}
