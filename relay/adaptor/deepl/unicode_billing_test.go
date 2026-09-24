package deepl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestSourceCharacterUsage exercises request conversion and both response paths
// with Unicode code points rather than UTF-8 byte lengths or grapheme counts.
// Source: https://developers.deepl.com/docs/resources/usage-limits
func TestSourceCharacterUsage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text       string
		characters int
	}{
		{"hello", 5}, {"你好", 2}, {"é", 1}, {"e\u0301", 2}, {"😀", 1}, {"A中😀", 3}, {"", 0},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(tc.text+"/"+map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
				t.Parallel()
				c, _ := gin.CreateTestContext(&closeAwareRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)})
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				a := &Adaptor{}
				_, err := a.ConvertRequest(c, 0, &model.GeneralOpenAIRequest{Model: "deepl-en", Messages: []model.Message{{Role: "user", Content: tc.text}}})
				require.NoError(t, err)
				resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"translations":[{"text":"translated","detected_source_language":"ZH"}]}`))}
				usage, apiErr := a.DoResponse(c, resp, &meta.Meta{ActualModelName: "deepl-en", IsStream: stream})
				require.Nil(t, apiErr)
				require.Equal(t, tc.characters, usage.PromptTokens)
				require.Equal(t, tc.characters, usage.TotalTokens)
				require.Zero(t, usage.CompletionTokens)
			})
		}
	}
}

// closeAwareRecorder implements the HTTP streaming contract without closing a
// client connection. httptest.ResponseRecorder alone lacks CloseNotifier.
type closeAwareRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

// CloseNotify returns the simulated client-disconnect channel.
func (r *closeAwareRecorder) CloseNotify() <-chan bool { return r.closed }
