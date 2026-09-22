package cohere

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// embeddingTestContext constructs an authenticated-contract fixture without external calls.
func embeddingTestContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// TestProtocolAuditCohereEmbeddings is also executed against the old implementation
// as a negative control: the prior adaptor routed embeddings through v1/chat.
func TestProtocolAuditCohereEmbeddings(t *testing.T) {
	for _, name := range []string{"embed-v4.0", "embed-english-v3.0", "embed-english-light-v3.0", "embed-multilingual-v3.0", "embed-multilingual-light-v3.0"} {
		t.Run(name, func(t *testing.T) {
			c, _ := embeddingTestContext(t, `{"model":"public-alias","input":["one","two"],"input_type":"search_query","truncate":"NONE","priority":0}`)
			a := &Adaptor{}
			req := &model.GeneralOpenAIRequest{Model: name, Input: []any{"one", "two"}}
			converted, err := a.ConvertRequest(c, relaymode.Embeddings, req)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.Equal(t, name, wire["model"])
			require.Equal(t, []any{"one", "two"}, wire["texts"])
			require.Equal(t, "search_query", wire["input_type"])
			require.Equal(t, float64(0), wire["priority"])
			require.Equal(t, "NONE", wire["truncate"])
			require.NotContains(t, wire, "input")
			require.NotContains(t, wire, "messages")
			url, err := a.GetRequestURL(&meta.Meta{Mode: relaymode.Embeddings, BaseURL: "https://api.cohere.com"})
			require.NoError(t, err)
			require.Equal(t, "https://api.cohere.com/v2/embed", url)
			require.True(t, channeltype.IsEndpointSupported(relaymode.Embeddings, channeltype.DefaultEndpointsForChannelType(channeltype.Cohere)))
			require.Equal(t, []any{"one", "two"}, req.Input)
		})
	}
}

// TestCohereEmbeddingValidation keeps invalid shapes out of paid provider requests.
func TestCohereEmbeddingValidation(t *testing.T) {
	for _, tc := range []struct {
		name              string
		input             any
		dimension         int
		encoding, options string
	}{
		{"nil", nil, 0, "", "{}"}, {"empty", []any{}, 0, "", "{}"},
		{"tokens", []any{1., 2.}, 0, "", "{}"}, {"mixed", []any{"text", 1.}, 0, "", "{}"},
		{"blank", " ", 0, "", "{}"}, {"too_many", make([]string, 97), 0, "", "{}"},
		{"dimension", "text", 333, "", "{}"}, {"encoding", "text", 0, "int8", "{}"},
		{"task", "text", 0, "", `{"input_type":"image"}`},
		{"truncate", "text", 0, "", `{"truncate":"invalid"}`},
		{"priority", "text", 0, "", `{"priority":-1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := embeddingTestContext(t, tc.options)
			_, err := (&Adaptor{}).ConvertRequest(c, relaymode.Embeddings, &model.GeneralOpenAIRequest{Model: "embed-v4.0", Input: tc.input, Dimensions: tc.dimension, EncodingFormat: tc.encoding})
			require.Error(t, err)
		})
	}
}

// TestCohereEmbeddingResponse verifies billed-unit precedence, output order,
// alias preservation and standard little-endian float32 base64 encoding.
func TestCohereEmbeddingResponse(t *testing.T) {
	for _, encoding := range []string{"float", "base64"} {
		t.Run(encoding, func(t *testing.T) {
			c, w := embeddingTestContext(t, `{}`)
			_, err := (&Adaptor{}).ConvertRequest(c, relaymode.Embeddings, &model.GeneralOpenAIRequest{Model: "embed-v4.0", Input: []string{"first", "second"}, EncodingFormat: encoding})
			require.NoError(t, err)
			resp := &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"provider-id"}}, Body: io.NopCloser(strings.NewReader(`{"embeddings":{"float":[[1,-2.5],[3,4]]},"meta":{"billed_units":{"input_tokens":7},"tokens":{"input_tokens":999}}}`))}
			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: relaymode.Embeddings, OriginModelName: "public", ActualModelName: "embed-v4.0", PromptTokens: 123})
			require.Nil(t, apiErr)
			require.Equal(t, 7, usage.PromptTokens)
			require.Equal(t, 7, usage.TotalTokens)
			require.Equal(t, "provider-id", w.Header().Get("X-Request-ID"))
			var result struct {
				Model string `json:"model"`
				Data  []struct {
					Index     int `json:"index"`
					Embedding any `json:"embedding"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Equal(t, "public", result.Model)
			require.Len(t, result.Data, 2)
			require.Equal(t, 1, result.Data[1].Index)
			if encoding == "base64" {
				encoded, ok := result.Data[0].Embedding.(string)
				require.True(t, ok)
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				require.NoError(t, err)
				require.Len(t, decoded, 8)
				require.Equal(t, float32(-2.5), math.Float32frombits(binary.LittleEndian.Uint32(decoded[4:])))
			} else {
				require.Equal(t, []any{float64(1), -2.5}, result.Data[0].Embedding)
			}
		})
	}
}

// TestCohereEmbeddingBadReceipts distinguishes provider rejection from accepted
// output with damaged shape, retaining available usage in the latter case.
func TestCohereEmbeddingBadReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantUsage  bool
	}{
		{"http_error", `{"message":"rate limited"}`, 429, false},
		{"success_error", `{"message":"invalid input"}`, 200, false},
		{"missing_vectors", `{"meta":{"billed_units":{"input_tokens":9}}}`, 200, true},
		{"negative_receipt", `{"embeddings":{"float":[[1]]},"meta":{"billed_units":{"input_tokens":-1}}}`, 200, false},
		{"mismatched_vectors", `{"embeddings":{"float":[[1],[2,3]]},"meta":{"billed_units":{"input_tokens":9}}}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := embeddingTestContext(t, `{}`)
			usage, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, &meta.Meta{Mode: relaymode.Embeddings})
			require.NotNil(t, apiErr)
			require.GreaterOrEqual(t, apiErr.StatusCode, 400)
			require.Equal(t, tc.wantUsage, usage != nil)
		})
	}
}
