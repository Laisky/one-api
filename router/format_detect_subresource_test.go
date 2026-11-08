package router

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestFormatAutoDetectNeverReroutesSubResources drives requests at
// sub-resources of the generation endpoints (Responses retrieval/cancel,
// Messages sub-paths) through the shipped router with a body shaped like a
// different generation format. Format auto-detection applies only to the
// generation endpoints themselves, so none of them may be re-dispatched as a
// paid generation or move any ledger.
func TestFormatAutoDetectNeverReroutesSubResources(t *testing.T) {
	// The Claude body carries a tool_result block and the Responses body an
	// input without messages, so DetectFormat classifies each unambiguously.
	claudeBody := countTokensWithMaxTokensBody
	responsesBody := `{"model":"` + countTokensClaudeModel + `","input":"hello","max_output_tokens":16}`
	for path, body := range map[string]string{
		"/v1/responses/resp_missing/cancel":     claudeBody,
		"/v1/responses/resp_missing/":           claudeBody,
		"/v1/messages/batches":                  responsesBody,
		"/v1/chat/completions/chatcmpl_missing": claudeBody,
	} {
		t.Run(strings.Trim(strings.ReplaceAll(path, "/", "_"), "_"), func(t *testing.T) {
			upstream, capture := newFakeAnthropicUpstream(t, nil)
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
			client.HTTPClient = upstream.Client()
			userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

			w := postClaudeJSON(t, engine, key, path, body, nil)

			userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
			require.Empty(t, capture.paths(), "a sub-resource request must never dispatch upstream work (status %d, body %s)", w.Code, w.Body.String())
			require.Equal(t, userBefore, userAfter, "no user quota may move")
			require.Equal(t, tokenBefore, tokenAfter, "no token quota may move")
			require.Equal(t, logsBefore, logsAfter, "no consume log may be written")
			require.Equal(t, costsBefore, costsAfter, "no request cost may be recorded")
			require.NotEqual(t, http.StatusOK, w.Code, "the sub-resource does not exist")
			t.Logf("status=%d body=%s", w.Code, w.Body.String())
		})
	}
}
