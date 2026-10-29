package vertexai

import (
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestVertexNanoImageInputs checks mapped Nano image errors and valid input conversion.
func TestVertexNanoImageInputs(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		valid     bool
	}{
		{"invalid", "data:invalid", false},
		{"bad_base64", "data:image/png;base64,%%%", false},
		{"valid", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
			req := &model.GeneralOpenAIRequest{Model: "alias", Messages: []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: tc.url}}}}}}
			got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, got)
				return
			}
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}
