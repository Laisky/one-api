package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThinking_JSONSerialization_AdaptiveOmitsBudgetTokens(t *testing.T) {
	thinking := Thinking{
		Type: "adaptive",
	}

	data, err := json.Marshal(thinking)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"type":"adaptive"`)
	assert.NotContains(t, string(data), "budget_tokens",
		"adaptive thinking must not serialize budget_tokens")
}

func TestThinking_JSONSerialization_EnabledIncludesBudgetTokens(t *testing.T) {
	thinking := Thinking{
		Type:         "enabled",
		BudgetTokens: IntPtr(4096),
	}

	data, err := json.Marshal(thinking)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"type":"enabled"`)
	assert.Contains(t, string(data), `"budget_tokens":4096`)
}

func TestThinking_JSONSerialization_EnabledNilBudgetOmitted(t *testing.T) {
	thinking := Thinking{
		Type: "enabled",
	}

	data, err := json.Marshal(thinking)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"type":"enabled"`)
	assert.NotContains(t, string(data), "budget_tokens",
		"nil budget_tokens must not appear in JSON")
}

func TestThinking_JSONDeserialization_AdaptiveNoBudget(t *testing.T) {
	input := `{"type":"adaptive"}`
	var thinking Thinking
	err := json.Unmarshal([]byte(input), &thinking)
	require.NoError(t, err)

	assert.Equal(t, "adaptive", thinking.Type)
	assert.Nil(t, thinking.BudgetTokens)
}

func TestThinking_JSONDeserialization_EnabledWithBudget(t *testing.T) {
	input := `{"type":"enabled","budget_tokens":2048}`
	var thinking Thinking
	err := json.Unmarshal([]byte(input), &thinking)
	require.NoError(t, err)

	assert.Equal(t, "enabled", thinking.Type)
	require.NotNil(t, thinking.BudgetTokens)
	assert.Equal(t, 2048, *thinking.BudgetTokens)
}

func TestThinking_JSONRoundtrip_Adaptive(t *testing.T) {
	original := Thinking{Type: "adaptive"}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Thinking
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "adaptive", decoded.Type)
	assert.Nil(t, decoded.BudgetTokens)
}

func TestThinking_JSONRoundtrip_Enabled(t *testing.T) {
	original := Thinking{Type: "enabled", BudgetTokens: IntPtr(8192)}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Thinking
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "enabled", decoded.Type)
	require.NotNil(t, decoded.BudgetTokens)
	assert.Equal(t, 8192, *decoded.BudgetTokens)
}

func TestIntPtr(t *testing.T) {
	p := IntPtr(42)
	require.NotNil(t, p)
	assert.Equal(t, 42, *p)
}
