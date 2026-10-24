package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
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
		meta: metalib.Meta{ActualModelName: "gpt-5"},
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

	// Missing and model-mismatched usage is never silently taken as free.
	f.observe([]byte(`{"type":"response.completed","response":{"id":"missing","usage":{"input_tokens":8}}}`))
	_, err = f.inherited(context.Background(), "missing")
	require.ErrorIs(t, err, errResponseContinuationUnpriceable)
	f.observe([]byte(`{"type":"response.completed","response":{"id":"wrong-model","model":"gpt-other","usage":{"input_tokens":8,"output_tokens":2}}}`))
	_, err = f.inherited(context.Background(), "wrong-model")
	require.ErrorIs(t, err, errResponseContinuationUnpriceable)
}
