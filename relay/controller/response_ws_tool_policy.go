package controller

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/tooling"
)

// responseWSToolFrame is the validation-only view of a client frame. Input stays
// raw so discovery inspection cannot round numbers or rewrite provider fields.
type responseWSToolFrame struct {
	Type  string                   `json:"type"`
	Tools []openai.ResponseAPITool `json:"tools"`
	Input json.RawMessage          `json:"input"`
}

// responseWebSocketToolAdmission combines tiered context and DeepSeek native
// tool holds into ONE atomic pre-dispatch debit. It snapshots the request's
// authenticated pricing/identity, then runs only on the client-reader goroutine.
// The optional tier funding argument preserves the old tool-only call contract.
func responseWebSocketToolAdmission(c *gin.Context, meta *metalib.Meta, funded *atomic.Int64, tierFunding ...*responseWSTierFunding) openai.ResponseAPIWebSocketAdmission {
	policyMeta := metalib.Meta{
		ChannelType:     meta.ChannelType,
		ActualModelName: meta.ActualModelName,
		OriginModelName: meta.OriginModelName,
		StartTime:       meta.StartTime,
		Mode:            meta.Mode,
	}
	provider := resolvePricingAdaptor(meta)
	var channel *model.Channel
	if raw, ok := c.Get(ctxkey.ChannelModel); ok {
		if record, ok := raw.(*model.Channel); ok && record != nil {
			channel = &model.Channel{Id: record.Id, Type: record.Type, Config: record.Config}
		}
	}
	userID, tokenID := meta.UserId, meta.TokenId
	var totalFunded int64
	var tier *responseWSTierFunding
	if len(tierFunding) > 0 {
		tier = tierFunding[0]
	}

	return func(ctx context.Context, frame []byte, upstreamURL string) error {
		// Quoting has no side effects. A malformed/unpriceable frame or invalid
		// tool declaration must not retain an unrelated partial reservation.
		var tierAdded, turnQuote int64
		if tier != nil {
			var err error
			tierAdded, turnQuote, err = tier.additionalQuote(ctx, frame)
			if err != nil {
				status := http.StatusBadRequest
				code := "invalid_response_tier_quote"
				if errors.Is(err, errResponseContinuationStoreUnavailable) {
					status, code = http.StatusServiceUnavailable, codeStateStoreUnavailable
				}
				return &openai.ResponseAPIWSAdmissionFundingError{Code: code, StatusCode: status, Cause: err}
			}
		}

		var toolAdded int64
		current := policyMeta
		current.BaseURL = upstreamURL
		if deepseekcompat.UsesDeepSeekToolReceiptContract(&current) {
			var request responseWSToolFrame
			if err := common.ValidateUnambiguousJSONRootKeys(frame, request); err != nil {
				return errors.Wrap(err, "validate response websocket tool keys")
			}
			if err := json.Unmarshal(frame, &request); err != nil {
				return errors.Wrap(err, "decode response websocket tool declarations")
			}
			if request.Type != "response.create" {
				return nil
			}
			items, err := openai.InspectResponseToolSearchInput(request.Input)
			if err != nil {
				return errors.Wrap(err, "inspect response websocket discovery history")
			}
			if err := validateResponseToolSearchItems(items); err != nil {
				return err
			}
			requested := make(map[string]struct{})
			for _, tool := range request.Tools {
				kind := strings.ToLower(strings.TrimSpace(tool.Type))
				if kind == "namespace" || kind == "tool_search" {
					if err := validateNativeResponseToolBilling([]openai.ResponseAPITool{tool}); err != nil {
						return err
					}
					continue
				}
				_, canonical := tooling.ClassifyClaudeTool(kind)
				if canonical == tooling.BuiltinWebSearch || canonical == tooling.BuiltinWebFetch {
					requested[canonical] = struct{}{}
				}
			}
			quotes, err := tooling.BuiltinToolQuotes(ctx, current.ActualModelName, &current, channel, provider, requested)
			if err != nil {
				return err
			}
			for name, price := range quotes {
				if name != tooling.BuiltinWebSearch && name != tooling.BuiltinWebFetch {
					continue
				}
				cost := tooling.SaturatingToolAllowance(price, tooling.ConservativeDeepSeekDefaultCalls)
				if cost > math.MaxInt64-toolAdded {
					return errors.New("conservative WebSocket tool quote overflows")
				}
				toolAdded += cost
			}
		}

		if tierAdded < 0 || toolAdded < 0 || tierAdded > math.MaxInt64-toolAdded {
			return errors.New("WebSocket combined reservation overflows")
		}
		added := tierAdded + toolAdded
		if added > math.MaxInt64-totalFunded {
			return errors.New("cumulative WebSocket reservation overflows")
		}
		if added > 0 {
			if userID <= 0 || tokenID <= 0 {
				return errors.New("WebSocket reservation requires an authenticated owner and token")
			}
			// Match the established Responses 403 for known owner exhaustion.
			// The subsequent database transaction still makes admission atomic
			// against concurrent sessions and their changing balances.
			if tierAdded > 0 {
				available, err := model.GetUserQuota(userID)
				if err != nil {
					return &openai.ResponseAPIWSAdmissionFundingError{Code: "get_user_quota_failed", StatusCode: http.StatusInternalServerError, Cause: err}
				}
				if available < added {
					return &openai.ResponseAPIWSAdmissionFundingError{Code: "insufficient_user_quota", StatusCode: http.StatusForbidden, Cause: errors.New("user quota is not enough")}
				}
			}
			if err := model.PreConsumeTokenQuota(ctx, tokenID, added); err != nil {
				if tierAdded > 0 {
					return &openai.ResponseAPIWSAdmissionFundingError{Code: "pre_consume_token_quota_failed", StatusCode: http.StatusForbidden, Cause: err}
				}
				return errors.Wrap(err, "atomically reserve conservative WebSocket tool quota")
			}
			syncUserQuotaCacheAfterPreConsume(ctx, userID, added, "response_websocket_frame_reservation")
			totalFunded += added
			if funded != nil {
				funded.Store(totalFunded)
			}
		}
		// No reservation state is committed until all validation succeeds,
		// even if the handshake already provided enough quota for this turn.
		if tier != nil {
			tier.accepted(turnQuote, tierAdded)
		}
		return nil
	}
}
