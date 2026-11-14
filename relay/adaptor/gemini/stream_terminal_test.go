package gemini

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestStreamResponseGeminiChat2OpenAI_EmptyParts tests that streamResponseGeminiChat2OpenAI
// handles edge cases properly without panicking.
func TestStreamResponseGeminiChat2OpenAI_EmptyParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    ChatResponse
		expectedNil bool
	}{
		{
			name: "no candidates should return nil",
			response: ChatResponse{
				Candidates: []ChatCandidate{},
			},
			expectedNil: true,
		},
		{
			name: "candidate with empty parts and terminal reason should emit finish",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role:  "model",
							Parts: []Part{},
						},
						FinishReason: "STOP",
					},
				},
			},
			expectedNil: false,
		},
		{
			name: "candidate with text part should return response",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role: "model",
							Parts: []Part{
								{Text: "Hello!"},
							},
						},
						FinishReason: "STOP",
					},
				},
			},
			expectedNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &mockResponseWriter{}
			c, _ := gin.CreateTestContext(w)

			// This should not panic
			result := streamResponseGeminiChat2OpenAI(c, &tt.response)

			if tt.expectedNil {
				require.Nil(t, result, "expected nil response for edge case")
			} else {
				require.NotNil(t, result, "expected non-nil response")
				if len(tt.response.Candidates[0].Content.Parts) == 0 {
					require.Equal(t, "stop", *result.Choices[0].FinishReason)
				}
			}
		})
	}
}
