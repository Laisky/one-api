package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSearchReceiptIsServerOnly excludes billable search receipts from public JSON.
func TestSearchReceiptIsServerOnly(t *testing.T) {
	var inbound Usage
	require.NoError(t, json.Unmarshal([]byte(`{"BilledSearchUnits":1,"billed_search_units":1,"prompt_tokens":7}`), &inbound))
	require.Nil(t, inbound.BilledSearchUnits)
	require.Equal(t, 7, inbound.PromptTokens)
	units := int64(3)
	encoded, err := json.Marshal(Usage{BilledSearchUnits: &units, PromptTokens: 7})
	require.NoError(t, err)
	require.JSONEq(t, `{"prompt_tokens":7}`, string(encoded))
}
