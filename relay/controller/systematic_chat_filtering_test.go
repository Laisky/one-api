package controller

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestSystematicChatFilteringSurvivesPassthrough checks that removed protocol
// fields cannot be restored as unknown extensions or nested overrides.
// Parameters: t runs the cases. Returns: none; all checks inspect serialized JSON.
func TestSystematicChatFilteringSurvivesPassthrough(t *testing.T) {
	original := []byte(`{"model":"custom","stop":["END"],"temperature":0.5,"top_k":10,"reasoning_effort":"high","extra_body":{"top_k":20},"vendor_extension":{"counter":9007199254740993}}`)
	updated := []byte(`{"model":"custom"}`)
	got, stats, _, err := mergeControlledPassthroughJSON(original, updated, true)
	require.NoError(t, err)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got, &root))
	for _, key := range []string{"stop", "temperature", "top_k", "reasoning_effort", "extra_body"} {
		require.NotContains(t, root, key)
	}
	require.Contains(t, string(root["vendor_extension"]), "9007199254740993")
	require.Equal(t, 1, stats.UnknownPreserved)

	// A provider extension not supplied as an explicit protocol field remains
	// eligible for the existing allowlisted extra_body forwarding mechanism.
	got, _, _, err = mergeControlledPassthroughJSON([]byte(`{"model":"custom","extra_body":{"top_k":20}}`), updated, true)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"custom","top_k":20}`, string(got))
}

// systematicFilteringAdaptor models a converter that deliberately removes
// unsupported protocol fields while keeping the normal adaptor interface.
type systematicFilteringAdaptor struct {
	openai.Adaptor
}

// ConvertRequest removes the fields exercised by the controller boundary test.
// Parameters: c and mode are unused; request is the input DTO to normalize.
// Returns: the filtered DTO and nil without contacting an inference provider.
func (a *systematicFilteringAdaptor) ConvertRequest(_ *gin.Context, _ int, request *relaymodel.GeneralOpenAIRequest) (any, error) {
	request.Stop = nil
	request.Temperature = nil
	request.TopK = nil
	request.ReasoningEffort = nil
	return request, nil
}

// TestSystematicChatRequestBodyPreservesFiltering exercises the actual chat
// request-body builder rather than only the merge helper. Parameters: t controls
// setup and assertions. Returns: none; no network or billable API is used.
func TestSystematicChatRequestBodyPreservesFiltering(t *testing.T) {
	previous := config.EnforceIncludeUsage
	config.EnforceIncludeUsage = false
	t.Cleanup(func() { config.EnforceIncludeUsage = previous })

	raw := `{"model":"custom","messages":[{"role":"user","content":"test"}],"stop":["END"],"temperature":0.5,"top_k":10,"reasoning_effort":"high","vendor_extension":{"counter":9007199254740993}}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
	ctx.Request.Header.Set("Content-Type", "application/json")
	var request relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	meta := &metalib.Meta{
		APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI,
		OriginModelName: "custom", ActualModelName: "custom", Mode: relaymode.ChatCompletions,
	}
	body, err := getRequestBody(ctx, meta, &request, &systematicFilteringAdaptor{}, false)
	require.NoError(t, err)
	wire, err := io.ReadAll(body)
	require.NoError(t, err)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &root))
	for _, key := range []string{"stop", "temperature", "top_k", "reasoning_effort"} {
		require.NotContains(t, root, key)
	}
	require.Contains(t, string(root["vendor_extension"]), "9007199254740993")
	require.Contains(t, root, "messages")
}
