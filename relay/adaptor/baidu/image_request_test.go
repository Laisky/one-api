package baidu

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertImageRequestDropsGPTImageControls checks that Baidu image
// requests never carry the OpenAI-only GPT Image controls, including the
// stream options, so a client's stream=true keeps Baidu's JSON response.
// Parameters: t runs the case. Returns: none.
func TestConvertImageRequestDropsGPTImageControls(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	got, err := (&Adaptor{}).ConvertImageRequest(c, &model.ImageRequest{
		Model: "irag-1.0", Prompt: "A lighthouse", OutputFormat: "png", Complexity: "high",
		SafetyIdentifier: "user", Stream: true, PartialImages: new(2),
	})
	require.NoError(t, err)
	require.Equal(t, &model.ImageRequest{Model: "irag-1.0", Prompt: "A lighthouse"}, got)
}
