package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
)

func TestResponseTierInheritedTokensRequiresExplicitUsage(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{name: "complete", raw: `{"input_tokens":7,"output_tokens":3}`, want: 10, ok: true},
		{name: "absent", raw: `{}`},
		{name: "missing output", raw: `{"input_tokens":7}`},
		{name: "missing input", raw: `{"output_tokens":3}`},
		{name: "zero output is ambiguous", raw: `{"input_tokens":7,"output_tokens":0}`},
		{name: "negative input", raw: `{"input_tokens":-5,"output_tokens":3}`},
		{name: "invalid JSON", raw: `{"input_tokens":12,`},
		{name: "overflow", raw: fmt.Sprintf(`{"input_tokens":%d,"output_tokens":1}`, math.MaxInt)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := responseTierInheritedTokens(json.RawMessage(tt.raw))
			if tt.ok {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestResponseWSTierFundingRejectsAmbiguousPricingKeys(t *testing.T) {
	// These wire frames can be priced against a different value from the one
	// a provider executes if a decoder chooses the first, rather than last,
	// occurrence. Unknown provider extensions remain untouched.
	for _, raw := range []string{
		`{"type":"response.create","input":"long","input":"short"}`,
		`{"type":"response.create","input":"short","Input":"long"}`,
		`{"type":"response.create","input":"hello","max_output_tokens":10,"max_output_tokens":10000}`,
		`{"type":"response.create","input":"hello","Max_Output_Tokens":10000}`,
	} {
		t.Run(raw, func(t *testing.T) {
			f := &responseWSTierFunding{enabled: true}
			added, quoted, err := f.additionalQuote(context.Background(), []byte(raw))
			require.ErrorIs(t, err, common.ErrAmbiguousJSONKey)
			require.Zero(t, added)
			require.Zero(t, quoted)
			require.Zero(t, f.reserved, "failed quote must not reserve funds")
			require.Zero(t, f.quoted, "failed quote must not commit a turn")
		})
	}
}

func TestResponseWSTierFundingObservedParentReceipts(t *testing.T) {
	f := &responseWSTierFunding{
		enabled: true,
		meta:    metalib.Meta{ActualModelName: "gpt-5"},
	}
	// Native collector historically accepts response.completed with omitted
	// status; every funded continuation must preserve that compatibility.
	f.observe([]byte(`{"type":"response.completed","response":{"id":"legacy","usage":{"input_tokens":8,"output_tokens":2}}}`))
	tokens, err := f.inherited(context.Background(), "legacy")
	require.NoError(t, err)
	require.Equal(t, 10, tokens)
	// A later duplicate must not overwrite the first completed usage receipt.
	f.observe([]byte(`{"type":"response.completed","response":{"id":"legacy","usage":{"input_tokens":1,"output_tokens":1}}}`))
	tokens, err = f.inherited(context.Background(), "legacy")
	require.NoError(t, err)
	require.Equal(t, 10, tokens)

	// Missing usage remains unpriceable, but a provider's canonical model
	// echo is not the authenticated model selection made by the proxy.
	f.observe([]byte(`{"type":"response.completed","response":{"id":"missing","usage":{"input_tokens":8}}}`))
	_, err = f.inherited(context.Background(), "missing")
	require.ErrorIs(t, err, errResponseContinuationUnpriceable)
	f.observe([]byte(`{"type":"response.completed","response":{"id":"alias-echo","model":"gpt-5-2026-10-01","usage":{"input_tokens":8,"output_tokens":2}}}`))
	tokens, err = f.inherited(context.Background(), "alias-echo")
	require.NoError(t, err, "provider canonical model echo must not override handshake model authorization")
	require.Equal(t, 10, tokens)

	// Both the funding observer and ownership guard must recognize incomplete
	// terminal receipts. These requests may continue after a model output cap.
	f.observe([]byte(`{"type":"response.incomplete","response":{"id":"incomplete","status":"incomplete","usage":{"input_tokens":11,"output_tokens":4}}}`))
	tokens, err = f.inherited(context.Background(), "incomplete")
	require.NoError(t, err)
	require.Equal(t, 15, tokens)

	// A raw terminal receipt can prove an explicit zero output, unlike older
	// serialized state records whose omitempty fields lose zero-vs-absent.
	f.observe([]byte(`{"type":"response.completed","response":{"id":"warmup","status":"completed","usage":{"input_tokens":8,"output_tokens":0}}}`))
	tokens, err = f.inherited(context.Background(), "warmup")
	require.NoError(t, err)
	require.Equal(t, 8, tokens)
}

func TestResponseWSTierFundingIgnoresControlFrames(t *testing.T) {
	f := &responseWSTierFunding{enabled: true}
	for _, frame := range []string{
		`{"type":"response.cancel"}`,
		`{"type":"ping","extension":"preserve"}`,
		`{"type":"session.update"}`,
	} {
		added, quoted, err := f.additionalQuote(context.Background(), []byte(frame))
		require.NoError(t, err)
		require.Zero(t, added, "non-creation frames must not consume quota")
		require.Zero(t, quoted, "non-creation frames must not be priced")
		require.Zero(t, f.reserved)
		require.Zero(t, f.quoted)
	}
}

func TestResponseTierAdmissionPromptBudgetOpaqueInputs(t *testing.T) {
	const name = "opaque-input-context-598"
	cfg := map[string]model.ModelConfigLocal{name: {ContextLength: 10000}}
	tests := []struct {
		name    string
		request *openai.ResponseAPIRequest
		want    int
	}{
		{
			name:    "normal text unchanged",
			request: &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"hello"}},
			want:    25,
		},
		{
			name:    "remote stored prompt template",
			request: &openai.ResponseAPIRequest{Model: name, Prompt: &openai.ResponseAPIPrompt{Id: "prompt_remote"}},
			want:    10000,
		},
		{
			name: "provider-resolved uploaded file",
			request: &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_file", "file_id": "file_remote"}}},
			}},
			want: 10000,
		},
		{
			name: "raw file input survives source representation",
			request: &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{
				json.RawMessage(`{"type":"input_file","file_id":"file_remote"}`),
			}},
			want: 10000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := responseTierAdmissionPromptBudget(tt.request, 25, cfg, nil, time.Time{})
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
	_, err := responseTierAdmissionPromptBudget(&openai.ResponseAPIRequest{
		Model:  "unregistered-opaque-input-598",
		Prompt: &openai.ResponseAPIPrompt{Id: "prompt_remote"},
	}, 25, nil, nil, time.Time{})
	require.ErrorContains(t, err, "without a model context bound",
		"an opaque provider-side expansion must never be funded at only the tokenized ID length")
}
