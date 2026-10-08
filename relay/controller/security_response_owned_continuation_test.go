package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Laisky/errors/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/state"
	"github.com/stretchr/testify/require"
)

// nativeContinuationObservation captures the exact incremental provider request,
// owner/token balances and the provider's inherited full-context input count.
type nativeContinuationObservation struct {
	Path, Parent            string
	Incremental, FullPrompt int
	User, Token             int64
	Err                     error
}

// TestSecurityNativeResponseOwnedContinuationAdmission completes a real first
// native response and uses its owner-scoped stored ID for a same-provider second
// turn. Parameters: t owns SQLite, an in-memory state store and two loopback
// provider calls per scenario. Returns: none; inherited tier costs must be
// reserved before the second provider call rather than overspending balances.
func TestSecurityNativeResponseOwnedContinuationAdmission(t *testing.T) {
	const name = "native-owned-continuation-fixture"
	const output = 20
	config := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{
		InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 100,
	}}}
	for _, scenario := range []string{"owner_low", "token_low", "funded", "store_failure"} {
		t.Run(scenario, func(t *testing.T) {
			userBalance, tokenBalance := int64(10000), int64(10000)
			if scenario == "owner_low" {
				userBalance = 3456
			}
			if scenario == "token_low" {
				tokenBalance = 3456
			}
			xaiVideoSetup(t, userBalance, false)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", tokenBalance).Error)
			store := enableStateForTest(t)
			observed := make(chan nativeContinuationObservation, 2)
			var calls atomic.Int32
			priorInput := 0
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				round := calls.Add(1)
				got := nativeContinuationObservation{Path: r.URL.Path}
				var request openai.ResponseAPIRequest
				got.Err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request)
				if got.Err == nil {
					got.Incremental = getResponseAPIPromptTokens(r.Context(), &request)
					if request.PreviousResponseId != nil {
						got.Parent = *request.PreviousResponseId
					}
					if request.Model != name || request.MaxOutputTokens == nil || *request.MaxOutputTokens != output || round > 2 {
						got.Err = fmt.Errorf("invalid bounded native continuation request")
					}
				}
				got.FullPrompt = got.Incremental
				if round == 1 {
					priorInput = got.Incremental
					if got.Parent != "" {
						got.Err = fmt.Errorf("first round unexpectedly has a parent")
					}
				} else if round == 2 {
					if got.Parent != "resp_owned_first" {
						got.Err = fmt.Errorf("second round lacks the completed provider binding")
					}
					// The local provider retains the actual first request and its
					// metered output, as previous_response_id does in production.
					got.FullPrompt += priorInput + output
				}
				var user model.User
				var token model.Token
				if got.Err == nil {
					got.Err = model.DB.First(&user, fallbackUserID).Error
				}
				if got.Err == nil {
					got.Err = model.DB.First(&token, fallbackTokenID).Error
				}
				got.User, got.Token = user.Quota, token.RemainQuota
				observed <- got
				if got.Err != nil {
					http.Error(w, "invalid bounded owned continuation", http.StatusBadRequest)
					return
				}
				id := "resp_owned_first"
				if round == 2 {
					id = "resp_owned_second"
				}
				w.Header().Set("Content-Type", "application/json")
				receipt := map[string]any{
					"id": id, "object": "response", "status": "completed", "model": name,
					"output": []any{map[string]any{
						"type": "message", "id": "msg_" + id, "status": "completed", "role": "assistant",
						"content": []any{map[string]any{"type": "output_text", "text": strings.Repeat("answer ", 8), "annotations": []any{}}},
					}},
					"usage": map[string]int{"input_tokens": got.FullPrompt, "output_tokens": output, "total_tokens": got.FullPrompt + output},
				}
				if err := json.NewEncoder(w).Encode(receipt); err != nil {
					return
				}
			}))
			t.Cleanup(provider.Close)
			old := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = old })

			fields := map[string]any{"model": "alias", "input": strings.Repeat("input ", 200), "max_output_tokens": output, "store": true}
			firstBody, err := json.Marshal(fields)
			require.NoError(t, err)
			c, _, firstID := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", string(firstBody), provider.URL, userBalance, 1, false, &config)
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			c.Set(ctxkey.TokenQuota, tokenBalance)
			firstID += "-first"
			c.Set(ctxkey.RequestId, firstID)
			meta := metalib.GetByContext(c)
			meta.OriginModelName, meta.ActualModelName = name, name
			require.True(t, supportsNativeResponseAPI(meta))
			require.Nil(t, RelayResponseAPIHelper(c))
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load())
			first := <-observed
			require.NoError(t, first.Err)
			require.Empty(t, first.Parent)
			require.Equal(t, "/v1/responses", first.Path)
			require.False(t, meta.ResponseAPIFallback)
			owner := stateOwnerFromMeta(meta)
			record, err := store.GetResponse(context.Background(), owner, "resp_owned_first")
			require.NoError(t, err, "parent must originate from the completed real first provider call")
			require.Equal(t, state.StatusCompleted, record.Status)
			require.True(t, record.StoreMode)
			require.NotEmpty(t, record.InputItems)
			require.NotEmpty(t, record.OutputItems)
			require.NotNil(t, record.Binding)
			require.Equal(t, meta.ChannelId, record.Binding.ChannelID)
			require.Equal(t, meta.APIType, record.Binding.APIType)
			require.Equal(t, "resp_owned_first", record.Binding.UpstreamResponseID)
			var parentUsage openai.ResponseAPIUsage
			require.NoError(t, json.Unmarshal(record.Usage, &parentUsage))
			require.Equal(t, first.FullPrompt, parentUsage.InputTokens)
			require.Equal(t, output, parentUsage.OutputTokens)
			_, err = store.GetResponse(context.Background(), state.OwnerScope{UserID: owner.UserID + 1, TokenID: owner.TokenID}, "resp_owned_first")
			require.ErrorIs(t, err, state.ErrNotFound, "provider handle is not authorization for a different owner")
			channelRatio, completionRatio := getChannelRatios(c)
			charge := func(prompt int) int64 {
				return quotautil.Compute(quotautil.ComputeInput{
					Usage:     &relaymodel.Usage{PromptTokens: prompt, CompletionTokens: output},
					ModelName: name, ModelRatio: channelRatio[name], ChannelModelRatio: channelRatio,
					GroupRatio: 1, ChannelModelConfigs: getChannelModelConfigs(c), ChannelCompletionRatio: completionRatio,
					PricingAdaptor: &openai.Adaptor{}, RequestTime: meta.StartTime,
				}).TotalQuota
			}
			firstCharge := charge(first.FullPrompt)
			require.Equal(t, firstCharge, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			require.Equal(t, firstCharge, consumeLogQuota(t, firstID))
			require.Equal(t, userBalance-firstCharge, reloadUserQuota(t))

			fields["input"] = "next"
			fields["previous_response_id"] = record.GatewayResponseID
			secondBody, err := json.Marshal(fields)
			require.NoError(t, err)
			var beforeToken model.Token
			require.NoError(t, model.DB.First(&beforeToken, fallbackTokenID).Error)
			secondCtx, _, secondID := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", string(secondBody), provider.URL, reloadUserQuota(t), 1, false, &config)
			secondID += "-second"
			secondCtx.Set(ctxkey.RequestId, secondID)
			secondCtx.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			secondCtx.Set(ctxkey.TokenQuota, beforeToken.RemainQuota)
			secondMeta := metalib.GetByContext(secondCtx)
			secondMeta.OriginModelName, secondMeta.ActualModelName = name, name
			var failedStore *continuationLookupStore
			if scenario == "store_failure" {
				failedStore = &continuationLookupStore{ResponseStateStore: store, lookupErr: errors.New("local second parent read outage")}
				state.SetForTest(failedStore)
			}
			apiErr := RelayResponseAPIHelper(secondCtx)
			drainCriticalTasks(t)
			var finalToken model.Token
			require.NoError(t, model.DB.First(&finalToken, fallbackTokenID).Error)
			held := secondCtx.GetInt64(ctxkey.PreConsumedQuotaAmount)
			if calls.Load() == 2 {
				second := <-observed
				require.NoError(t, second.Err)
				require.Equal(t, "resp_owned_first", second.Parent)
				require.Equal(t, "/v1/responses", second.Path)
				require.Less(t, second.Incremental, 100)
				require.GreaterOrEqual(t, second.FullPrompt, 100)
				require.Equal(t, parentUsage.InputTokens+parentUsage.OutputTokens+second.Incremental, second.FullPrompt)
				require.False(t, secondMeta.ResponseAPIFallback, "same-provider continuation sends only incremental input")
				secondCharge := charge(second.FullPrompt)
				require.Equal(t, held, userBalance-firstCharge-second.User)
				require.Equal(t, held, tokenBalance-firstCharge-second.Token)
				require.Equal(t, secondCharge, consumeLogQuota(t, secondID))
				require.Equal(t, userBalance-firstCharge-secondCharge, reloadUserQuota(t))
				require.Equal(t, tokenBalance-firstCharge-secondCharge, finalToken.RemainQuota)
				t.Logf("OWNED_CONTINUATION scenario=%s parent_input=%d parent_output=%d incremental=%d full_context=%d first_hold=%d first_charge=%d second_hold=%d second_charge=%d calls=%d final_user=%d final_token=%d api_error=%v",
					scenario, parentUsage.InputTokens, parentUsage.OutputTokens, second.Incremental, second.FullPrompt, c.GetInt64(ctxkey.PreConsumedQuotaAmount), firstCharge, held, secondCharge, calls.Load(), reloadUserQuota(t), finalToken.RemainQuota, apiErr)
				if scenario == "funded" {
					require.Equal(t, held, secondCharge, "known inherited context must be held exactly once")
					require.Nil(t, apiErr)
					require.EqualValues(t, 2, calls.Load())
					require.Equal(t, second.Incremental, secondMeta.PromptTokens, "fallback usage remains the current-body estimate")
					require.Equal(t, firstCharge+secondCharge, finalToken.UsedQuota)
					child, childErr := store.GetResponse(context.Background(), owner, "resp_owned_second")
					require.NoError(t, childErr)
					require.Equal(t, "resp_owned_first", child.ParentResponseID)
				}
			}
			if scenario != "funded" {
				require.EqualValues(t, 1, calls.Load(), "unaffordable owned continuation must reject before second provider I/O")
				require.NotNil(t, apiErr)
				status := http.StatusForbidden
				if scenario == "store_failure" {
					status = http.StatusServiceUnavailable
					require.Equal(t, 1, failedStore.reads, "existing provider binding lookup succeeds; only the additional usage read fails")
					require.True(t, failedStore.bounded)
				}
				require.Equal(t, status, apiErr.StatusCode)
				require.Zero(t, held)
				require.Equal(t, userBalance-firstCharge, reloadUserQuota(t))
				require.Equal(t, tokenBalance-firstCharge, finalToken.RemainQuota)
				require.Equal(t, firstCharge, finalToken.UsedQuota)
			}
		})
	}
}
