package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/ctxkey"
	"math"
	"net/http"
	"testing"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
	"github.com/stretchr/testify/require"
)

// continuationLookupStore observes the candidate's single bounded owner lookup.
type continuationLookupStore struct {
	state.ResponseStateStore
	reads         int
	bounded       bool
	owner         state.OwnerScope
	usageOverride json.RawMessage
	lookupErr     error
}

// GetResponse records its context deadline and owner, then returns the underlying
// immutable response or store error for the requested ID.
func (s *continuationLookupStore) GetResponse(ctx context.Context, owner state.OwnerScope, id string) (*state.ResponseStateRecord, error) {
	s.reads++
	_, s.bounded = ctx.Deadline()
	s.owner = owner
	if s.lookupErr != nil {
		return nil, errors.WithStack(s.lookupErr)
	}
	record, err := s.ResponseStateStore.GetResponse(ctx, owner, id)
	if err == nil && record != nil && len(s.usageOverride) > 0 {
		record.Usage = s.usageOverride
	}
	return record, err
}

// TestSecurityResponseContinuationTierCounters retains known-counter arithmetic,
// bounded owner lookup, matching metadata, and the explicitly unresolved legacy
// receipt policy. Parameters: t owns isolated memory state. Returns: none;
// known counters augment only the quote and malformed legacy data remains legacy.
func TestSecurityResponseContinuationTierCounters(t *testing.T) {
	const name = "native-continuation-counter-fixture"
	for _, scenario := range []string{
		"known", "zero", "zero_output", "missing_usage", "null_usage", "malformed", "malformed_json", "negative_incremental",
		"store_failure", "free_group", "missing_input", "missing_output", "negative_input", "negative_output",
		"unrepresentable", "parent_overflow", "incremental_overflow", "different_model",
		"different_current_model", "different_channel", "different_api", "different_handle",
		"missing_handle", "missing_binding", "nonterminal", "incomplete", "cancelled", "failed",
		"no_parent", "no_handle", "foreign_owner", "missing_parent", "disabled", "flat",
	} {
		t.Run(scenario, func(t *testing.T) {
			local := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 100}}}
			if scenario == "flat" {
				local.Tiers = nil
			}
			c, _, _ := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", "{}", "", 10000, 1, false, &local)
			meta := metalib.GetByContext(c)
			meta.OriginModelName, meta.ActualModelName = name, name
			memory := enableStateForTest(t)
			observed := &continuationLookupStore{ResponseStateStore: memory}
			state.SetForTest(observed)
			owner := stateOwnerFromMeta(meta)
			previous := "upstream_parent"
			output := 20
			request := &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"next"}, MaxOutputTokens: &output, PreviousResponseId: &previous}
			c.Set(ctxNativeGatewayParent, "owned_parent")
			record := &state.ResponseStateRecord{
				GatewayResponseID: "owned_parent", Owner: owner, Status: state.StatusCompleted,
				StoreMode: true, Usage: json.RawMessage(`{"input_tokens":456,"output_tokens":20}`),
				Binding: &state.ProviderBinding{ChannelID: meta.ChannelId, APIType: meta.APIType, ActualModel: name, UpstreamResponseID: previous},
			}
			expected, expectedReads, expectError := 486, 1, false
			incremental := 10
			switch scenario {
			case "store_failure":
				observed.lookupErr = errors.New("local parent read outage")
				expectError = true
			case "free_group":
				c.Set(ctxkey.ChannelRatio, 0.0)
				observed.lookupErr = errors.New("unused parent read outage")
				expectedReads = 0
			case "zero":
				record.Usage = json.RawMessage(`{"input_tokens":0,"output_tokens":0}`)
				expected = 10
			case "zero_output":
				record.Usage = json.RawMessage(`{"input_tokens":456,"output_tokens":0}`)
				expected = 466
			case "missing_usage":
				record.Usage = nil
				expected = 10
			case "null_usage":
				record.Usage = json.RawMessage("null")
				expected = 10
			case "malformed":
				record.Usage = json.RawMessage(`{"input_tokens":"invalid","output_tokens":20}`)
				expected = 10
			case "malformed_json":
				observed.usageOverride = json.RawMessage("{")
				expected = 10
			case "negative_incremental":
				incremental = -1
				expectError = true
			case "missing_input":
				record.Usage = json.RawMessage(`{"output_tokens":20}`)
				expected = 10
			case "missing_output":
				record.Usage = json.RawMessage(`{"input_tokens":456}`)
				expected = 10
			case "negative_input":
				record.Usage = json.RawMessage(`{"input_tokens":-1,"output_tokens":20}`)
				expected = 10
			case "negative_output":
				record.Usage = json.RawMessage(`{"input_tokens":456,"output_tokens":-1}`)
				expected = 10
			case "unrepresentable":
				record.Usage = json.RawMessage(`{"input_tokens":999999999999999999999999,"output_tokens":20}`)
				expected = 10
			case "parent_overflow":
				record.Usage = json.RawMessage(fmt.Sprintf(`{"input_tokens":%d,"output_tokens":1}`, math.MaxInt))
				expectError = true
			case "incremental_overflow":
				record.Usage = json.RawMessage(fmt.Sprintf(`{"input_tokens":%d,"output_tokens":0}`, math.MaxInt))
				expectError = true
			case "different_model":
				record.Binding.ActualModel = "other-model"
				expected = 10
			case "different_current_model":
				meta.ActualModelName = "other-model"
				expected = 10
			case "different_channel":
				record.Binding.ChannelID++
				expected = 10
			case "different_api":
				record.Binding.APIType++
				expected = 10
			case "different_handle":
				record.Binding.UpstreamResponseID = "another_handle"
				expected = 10
			case "missing_handle":
				record.Binding.UpstreamResponseID = ""
				expected = 10
			case "missing_binding":
				record.Binding = nil
				expected = 10
			case "nonterminal":
				record.Status = state.StatusInProgress
				expected = 10
			case "incomplete":
				record.Status = state.StatusIncomplete
			case "cancelled":
				record.Status = state.StatusCancelled
			case "failed":
				record.Status = state.StatusFailed
			case "no_parent":
				c.Set(ctxNativeGatewayParent, "")
				expected, expectedReads = 10, 0
			case "no_handle":
				request.PreviousResponseId = nil
				expected, expectedReads = 10, 0
			case "foreign_owner":
				record.Owner.UserID++
				expected = 10
			case "missing_parent":
				c.Set(ctxNativeGatewayParent, "missing_parent")
				expected = 10
			case "disabled":
				state.SetForTest(nil)
				expected, expectedReads = 10, 0
			case "flat":
				expectedReads = 0
			}
			_, err := memory.CreateResponse(context.Background(), record, "fixture_parent")
			require.NoError(t, err)
			if scenario == "flat" || scenario == "free_group" {
				quote, applies, quoteErr := quoteResponseTierAdmission(c, meta, request, 10, nil)
				require.NoError(t, quoteErr)
				if scenario == "flat" {
					require.False(t, applies)
				} else {
					require.True(t, applies)
					require.Zero(t, quote, "free continuation needs no new parent usage lookup")
				}
			} else {
				prompt, quoteErr := responseContinuationTierPrompt(c, meta, request, incremental)
				if expectError {
					require.Error(t, quoteErr)
					if scenario == "store_failure" {
						require.ErrorIs(t, quoteErr, errResponseContinuationStoreUnavailable)
					}
				} else {
					require.NoError(t, quoteErr)
					require.Equal(t, expected, prompt)
				}
			}
			require.Equal(t, expectedReads, observed.reads, "one parent record, no chain traversal")
			if expectedReads > 0 {
				require.True(t, observed.bounded)
				require.Equal(t, owner, observed.owner)
			}
			if expectError {
				held, apiErr := preConsumeResponseAPIQuota(c, request, incremental, 1, 2, false, meta)
				require.Zero(t, held)
				require.NotNil(t, apiErr)
				status := http.StatusBadRequest
				if scenario == "store_failure" {
					status = http.StatusServiceUnavailable
				}
				require.Equal(t, status, apiErr.StatusCode, "operational state failure survives quote wrapping")
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			}
			if scenario == "known" {
				body, marshalErr := json.Marshal(request)
				require.NoError(t, marshalErr)
				quote, applies, quoteErr := quoteResponseTierAdmission(c, meta, request, 999, body)
				require.NoError(t, quoteErr)
				require.True(t, applies)
				require.EqualValues(t, 2486, quote)
				require.Equal(t, 10, meta.PromptTokens, "synthetic settlement still uses only actual current-body estimate")
				require.Equal(t, previous, *request.PreviousResponseId)
			}
		})
	}
}
