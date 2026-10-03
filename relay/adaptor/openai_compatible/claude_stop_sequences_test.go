package openai_compatible

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertClaudeRequestOmitsAbsentStopSequences pins the wire shape of the
// `stop` field.
//
// GeneralOpenAIRequest.Stop is declared `any`, and encoding/json's `omitempty`
// only omits a nil *interface* — an interface holding a nil []string is still
// emitted, as `null`. Assigning ClaudeRequest.StopSequences unconditionally put
// `"stop": null` on every converted request that carried no stop sequences, i.e.
// nearly all of them. Strict OpenAI-compatible upstreams reject that, and
// aws.ValidateUnsupportedParameters read the non-nil interface as "the caller
// sent a stop parameter" and returned a 400 for Bedrock models that do not
// support one.
func TestConvertClaudeRequestOmitsAbsentStopSequences(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name          string
		stopSequences []string
		wantPresent   bool
		wantValue     string
	}{
		{name: "absent", stopSequences: nil, wantPresent: false},
		{name: "explicitly empty", stopSequences: []string{}, wantPresent: false},
		{name: "populated", stopSequences: []string{"END"}, wantPresent: true, wantValue: `["END"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

			converted, err := ConvertClaudeRequest(c, &model.ClaudeRequest{
				Model:         "claude-opus-4-6",
				MaxTokens:     100,
				StopSequences: tc.stopSequences,
			})
			require.NoError(t, err)

			body, err := json.Marshal(converted)
			require.NoError(t, err)
			require.NotContains(t, string(body), `"stop":null`,
				"a nil slice inside an `any` field must be omitted, not serialized as null")

			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &fields))
			raw, present := fields["stop"]
			require.Equal(t, tc.wantPresent, present, "unexpected presence of `stop` in %s", body)
			if tc.wantPresent {
				require.JSONEq(t, tc.wantValue, string(raw))
			}
		})
	}
}
