package model

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
)

// TestDeepSeekReviewReasoningBinding checks the shared Responses reasoning
// schema through JSON binding, retaining rejection of unknown effort values.
func TestDeepSeekReviewReasoningBinding(t *testing.T) {
	t.Parallel()
	for _, effort := range []string{"none", "default", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "unknown"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"effort":"`+effort+`"}`))
			req.Header.Set("Content-Type", "application/json")
			var reasoning OpenAIResponseReasoning
			err := binding.JSON.Bind(req, &reasoning)
			if effort == "unknown" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, effort, *reasoning.Effort)
		})
	}
}
