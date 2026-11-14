package controller

import (
	"context"
	"net/http"

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
// returns the hydrated request with the fallback tool catalog built from it,
// which conversion and rendering reuse, or the existing state/admission error
// without dispatching upstream work. The limits do not depend on state
// persistence.
func prepareResponseFallbackInput(ctx context.Context, c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest) (*openai.ResponseAPIRequest, *openai.ResponseFallbackToolCatalog, *relaymodel.ErrorWithStatusCode) {
	if request == nil {
		return nil, nil, openai.ErrorWrapper(errors.New("response api request is nil"), "invalid_response_api_request", http.StatusBadRequest)
	}
	// Namespaces, custom tools and client tool search are emulated through the
	// fallback tool catalog, which keeps qualified call identities reversible.
	// Validate that the catalog is representable before state hydration or
	// quota reservation so an unrepresentable definition never reserves quota.
	if _, unsupported := buildResponseFallbackToolCatalog(request, responseFallbackConversionOptions(meta).PreserveJSONSchema); unsupported != nil {
		return nil, nil, unsupported
	}
	if err := openai.ValidateResponseAPIFallbackInput(request.Input); err != nil {
		return nil, nil, openai.ErrorWrapper(err, "response_fallback_input_too_large", http.StatusRequestEntityTooLarge)
	}

	if err := responseFallbackFidelityError(request.Input, true, isNativeGoogleChannel(meta)); err != nil {
		return nil, nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, openai.ErrorWrapper(err, "response_fallback_cancelled", http.StatusRequestTimeout)
	}

	hydrated, stateErr := hydrateResponseAPIRequestForFallback(ctx, meta, request, responseFallbackTarget(meta))
	if stateErr != nil {
		return nil, nil, stateErr
	}
	if hydrated == nil {
		return nil, nil, openai.ErrorWrapper(errors.New("response fallback hydration returned no request"), "response_fallback_hydration_failed", http.StatusInternalServerError)
	}
	// Hydration can add discovery history, so the catalog conversion and
	// rendering use is built once from the hydrated request.
	catalog, unsupported := buildResponseFallbackToolCatalog(hydrated, responseFallbackConversionOptions(meta).PreserveJSONSchema)
	if unsupported != nil {
		return nil, nil, unsupported
	}
	if err := openai.ValidateResponseAPIFallbackInput(hydrated.Input); err != nil {
		return nil, nil, openai.ErrorWrapper(err, "response_fallback_input_too_large", http.StatusRequestEntityTooLarge)
	}
	if err := responseFallbackFidelityError(hydrated.Input, false, isNativeGoogleChannel(meta)); err != nil {
		return nil, nil, err
	}
	// Hydration returns a clone; snapshot the original incremental request only
	// after both input forms pass admission, avoiding pending state on rejection.
	capturePendingStateCommit(c, meta, request)
	return hydrated, catalog, nil
}

// responseFallbackFidelityError maps shared semantic admission errors without exposing input values.
func responseFallbackFidelityError(input []any, allowReferences bool, nativeMedia ...bool) *relaymodel.ErrorWithStatusCode {
	err := openai.ValidateResponseAPIFallbackFidelity(input, allowReferences, nativeMedia...)
	if err == nil {
		return nil
	}
	code, status := "response_fallback_not_supported", http.StatusBadRequest
	if errors.Is(err, openai.ErrResponseFallbackStateNotPortable) {
		// Preserve the existing canonical-state portability error contract even
		// when admission rejects before the state hydrator runs.
		code, status = codeStateNotPortable, http.StatusConflict
	}
	return openai.ErrorWrapper(err, code, status)
}
