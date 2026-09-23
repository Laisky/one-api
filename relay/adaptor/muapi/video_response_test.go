package muapi

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMuAPIVideoResponseBindsAcceptedTask verifies that a native request_id
// is persisted through the provider-independent async task contract.
func TestMuAPIVideoResponseBindsAcceptedTask(t *testing.T) {
	t.Parallel()
	c, recorder := newMuAPITestContextWithRecorder(http.MethodPost, "/v1/videos", "")
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"job-123","status":"processing"}`)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.True(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, `{"request_id":"job-123","status":"processing"}`, recorder.Body.String())
}

// TestMuAPIVideoResponseForwardsPollingResult verifies completed results stay
// in MuAPI's native response shape and do not create another task binding.
func TestMuAPIVideoResponseForwardsPollingResult(t *testing.T) {
	t.Parallel()
	c, recorder := newMuAPITestContextWithRecorder(http.MethodGet, "/v1/videos/job-123", "")
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"status":"completed","outputs":[{"video_url":"https://cdn.example/video.mp4"}]}`)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.False(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
	require.JSONEq(t, `{"status":"completed","outputs":[{"video_url":"https://cdn.example/video.mp4"}]}`, recorder.Body.String())
}

// TestMuAPIVideoResponseRejectsMissingRequestID prevents billing an accepted
// creation whose task cannot be polled safely.
func TestMuAPIVideoResponseRejectsMissingRequestID(t *testing.T) {
	t.Parallel()
	c, _ := newMuAPITestContextWithRecorder(http.MethodPost, "/v1/videos", "")
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader(`{"status":"processing"}`)),
	}

	_, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	require.False(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
}
