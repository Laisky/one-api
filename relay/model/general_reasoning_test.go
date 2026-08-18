package model

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
)

func TestReasoningEffortBindingAcceptsDefault(t *testing.T) {
	t.Parallel()

	effort := "default"
	require.NoError(t, binding.Validator.ValidateStruct(&GeneralOpenAIRequest{
		ReasoningEffort: &effort,
	}))
	require.NoError(t, binding.Validator.ValidateStruct(&OpenAIResponseReasoning{
		Effort: &effort,
	}))
}

func TestReasoningEffortBindingRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	effort := "unexpected"
	require.Error(t, binding.Validator.ValidateStruct(&GeneralOpenAIRequest{
		ReasoningEffort: &effort,
	}))
	require.Error(t, binding.Validator.ValidateStruct(&OpenAIResponseReasoning{
		Effort: &effort,
	}))
}
