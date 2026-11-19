package muapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/model"
)

// TestPrepareVideoRequestAcceptsOpenAIStringSeconds verifies that the OpenAI
// contract's decimal-string `seconds` (already validated and billed by the
// gateway) reaches MuAPI as the numeric duration MuAPI's native API expects,
// while malformed strings and conflicting aliases are still refused.
func TestPrepareVideoRequestAcceptsOpenAIStringSeconds(t *testing.T) {
	t.Parallel()
	c := newMuAPITestContext(http.MethodPost, "/v1/async/videos", `{"model":"veo3-fast","prompt":"a lighthouse","seconds":"5"}`)
	request := &model.VideoRequest{Model: "veo3-fast", Seconds: float64Ptr(5)}
	_, err := (&Adaptor{}).PrepareVideoRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, request.Duration)
	require.Equal(t, float64(5), *request.Duration)
	body, err := common.GetRequestBody(c)
	require.NoError(t, err)
	require.JSONEq(t, `{"prompt":"a lighthouse","duration":5}`, string(body))

	for _, refused := range []string{
		`{"model":"veo3-fast","seconds":"5s"}`,
		`{"model":"veo3-fast","seconds":"-5"}`,
		`{"model":"veo3-fast","seconds":" 5"}`,
		`{"model":"veo3-fast","seconds":"1e1"}`,
		`{"model":"veo3-fast","seconds":"7.5"}`,
		`{"model":"veo3-fast","seconds":"5","duration":6}`,
	} {
		c := newMuAPITestContext(http.MethodPost, "/v1/async/videos", refused)
		_, err := (&Adaptor{}).PrepareVideoRequest(c, &model.VideoRequest{Model: "veo3-fast"})
		require.Error(t, err, refused)
	}
}
