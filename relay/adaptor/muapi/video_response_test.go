package muapi

import (
	"encoding/json"
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

// TestMuAPIVideoResponseForwardsSafeHeadersOnly keeps provider billing data
// internal while preserving headers useful to the client and transport.
func TestMuAPIVideoResponseForwardsSafeHeadersOnly(t *testing.T) {
	t.Parallel()
	c, recorder := newMuAPITestContextWithRecorder(http.MethodGet, "/v1/videos/job-123", "")
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":         []string{"application/json"},
			"Retry-After":          []string{"2"},
			"X-Request-Id":         []string{"request-123"},
			"X-MuAPI-Cost-USD":     []string{"0.12"},
			"X-MuAPI-Cost-Credits": []string{"12"},
		},
		Body: io.NopCloser(strings.NewReader(`{"status":"completed"}`)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, "2", recorder.Header().Get("Retry-After"))
	require.Equal(t, "request-123", recorder.Header().Get("X-Request-Id"))
	require.Empty(t, recorder.Header().Get("X-MuAPI-Cost-USD"))
	require.Empty(t, recorder.Header().Get("X-MuAPI-Cost-Credits"))
}

func TestHasMuAPIError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "missing", raw: nil, want: false},
		{name: "whitespace", raw: json.RawMessage("  \t\n"), want: false},
		{name: "null", raw: json.RawMessage("null"), want: false},
		{name: "empty string", raw: json.RawMessage(`""`), want: false},
		{name: "whitespace string", raw: json.RawMessage(`"  \t"`), want: false},
		{name: "message", raw: json.RawMessage(`"provider failed"`), want: true},
		{name: "false", raw: json.RawMessage("false"), want: true},
		{name: "object", raw: json.RawMessage("{}"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, hasMuAPIError(test.raw))
		})
	}
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
