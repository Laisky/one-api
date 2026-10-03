package controller

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/jina"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// textOnlyRerankProbe detects unsafe conversion into an adaptor lacking native-input support.
type textOnlyRerankProbe struct {
	adaptor.Adaptor
	called bool
}

// GetChannelName identifies the probe without invoking the embedded nil adaptor.
func (a *textOnlyRerankProbe) GetChannelName() string { return "text-only-probe" }

// ConvertRerankRequest records calls and otherwise preserves canonical text requests.
func (a *textOnlyRerankProbe) ConvertRerankRequest(_ *gin.Context, r *model.RerankRequest) (any, error) {
	a.called = true
	return r, nil
}

// TestJinaRerankPipeline verifies canonical decode, mapping, native options and fallback safety.
func TestJinaRerankPipeline(t *testing.T) {
	t.Parallel()
	raw := `{"model":"public-alias","query":"query","documents":[{"text":"document"},{"image":"d.png"}],"return_documents":false}`
	var request model.RerankRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	require.NoError(t, request.Normalize())
	request.Model = "jina-reranker-m0"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/rerank", strings.NewReader(raw))
	m := &meta.Meta{ChannelType: channeltype.Jina, Mode: relaymode.Rerank}
	reader, err := prepareRerankRequestBody(c, m, &jina.Adaptor{}, &request)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"jina-reranker-m0","query":"query","documents":[{"text":"document"},{"image":"d.png"}],"return_documents":false}`, string(body))
	probe := &textOnlyRerankProbe{}
	_, err = prepareRerankRequestBody(c, m, probe, &request)
	require.ErrorContains(t, err, "structured rerank inputs are not supported")
	require.False(t, probe.called)
	_, err = prepareRerankRequestBody(c, m, probe, &model.RerankRequest{Query: "q", Documents: []string{"d"}})
	require.NoError(t, err)
	require.True(t, probe.called)
}

// TestJinaResponsesUsesChatBridge prevents accidental routing to a nonexistent native API.
func TestJinaResponsesUsesChatBridge(t *testing.T) {
	t.Parallel()
	require.False(t, supportsNativeResponseAPI(&meta.Meta{ChannelType: channeltype.Jina, ActualModelName: "jina-ocr-v1"}))
}
