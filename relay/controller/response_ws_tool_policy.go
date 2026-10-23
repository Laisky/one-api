package controller

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
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

// responseWebSocketToolAdmission snapshots c's channel policy and meta's pinned
// model on the request goroutine. It returns a validator that reuses the HTTP
// receipt policy for the actual DeepSeek WebSocket target without retaining c,
// changing accepted frame bytes, or changing unrelated providers' admission.
func responseWebSocketToolAdmission(c *gin.Context, meta *metalib.Meta) openai.ResponseAPIWebSocketAdmission {
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
	// Snapshot the remaining balance on the request goroutine; the socket
	// pump must never dereference *gin.Context after its handler has returned.
	var balance int64 = math.MaxInt64
	var balanceErr error
	if meta.UserId > 0 {
		balance, balanceErr = model.CacheGetUserQuota(gmw.Ctx(c), meta.UserId)
		if tokenQuota := c.GetInt64(ctxkey.TokenQuota); !c.GetBool(ctxkey.TokenQuotaUnlimited) && tokenQuota > 0 {
			balance = min(balance, tokenQuota)
		}
	}
	var checkedFloor int64

	return func(ctx context.Context, frame []byte, upstreamURL string) error {
		if balanceErr != nil {
			return errors.Wrap(balanceErr, "resolve websocket tool budget")
		}
		// Native WebSockets currently resolve BaseURL directly. The HTTP endpoint
		// override and a previous observed URL must not classify a different host.
		current := policyMeta
		current.BaseURL = upstreamURL
		if !deepseekcompat.UsesDeepSeekToolReceiptContract(&current) {
			return nil
		}

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
		var added int64
		for name, price := range quotes {
			if name != tooling.BuiltinWebSearch && name != tooling.BuiltinWebFetch {
				continue
			}
			cost := tooling.SaturatingToolAllowance(price, tooling.ConservativeDeepSeekDefaultCalls)
			if cost > math.MaxInt64-added {
				return errors.New("conservative WebSocket tool quote overflows")
			}
			added += cost
		}
		if checkedFloor > balance || added > balance-checkedFloor {
			return errors.New("insufficient quota for conservative WebSocket tool allowance")
		}
		checkedFloor += added
		return nil
	}
}
