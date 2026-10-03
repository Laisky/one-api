package zhipu

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertVoiceCloneRequestPassthrough verifies the native DTO is forwarded
// unchanged by the zhipu adaptor.
func TestConvertVoiceCloneRequestPassthrough(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	req := &model.VoiceCloneRequest{
		Model:     "glm-tts-clone",
		VoiceName: "my_custom_voice_001",
		Text:      "示例音频文本",
		Input:     "欢迎使用音色复刻服务",
		FileID:    "file_abc123",
		RequestID: "req_001",
	}

	c, _ := newVoiceCloneTestContext()
	convertedAny, err := a.ConvertVoiceCloneRequest(c, req)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.VoiceCloneRequest)
	require.True(t, ok)
	require.Equal(t, req, converted)
}

// TestDoVoiceCloneResponsePassesThrough verifies the success envelope
// (voice id + preview file id) is relayed verbatim.
func TestDoVoiceCloneResponsePassesThrough(t *testing.T) {
	t.Parallel()

	body := []byte(`{"voice":"voice_clone_20260315_143052_001","file_id":"file_xyz789","file_purpose":"voice-clone-output","request_id":"req_001"}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	c, w := newVoiceCloneTestContext()

	a := &Adaptor{}
	usage, bizErr := a.DoVoiceCloneResponse(c, resp, nil)
	require.Nil(t, bizErr)
	require.Nil(t, usage)
	require.Equal(t, http.StatusOK, c.Writer.Status())
	require.JSONEq(t, string(body), w.Body.String())
}

// TestDoVoiceCloneResponseDetectsZhipuError verifies the error envelope is
// surfaced as a business error with the upstream status.
func TestDoVoiceCloneResponseDetectsZhipuError(t *testing.T) {
	t.Parallel()

	body := []byte(`{"error":{"code":"1214","message":"voice id does not exist"}}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	c, _ := newVoiceCloneTestContext()

	a := &Adaptor{}
	usage, bizErr := a.DoVoiceCloneResponse(c, resp, nil)
	require.NotNil(t, bizErr)
	require.Nil(t, usage)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
	require.Equal(t, "voice id does not exist", bizErr.Error.Message)
}

func newVoiceCloneTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/voice/clones", nil)
	return c, w
}
