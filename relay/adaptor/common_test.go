package adaptor

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
)

// TestSetupCommonRequestHeader verifies common request headers and X-prefixed
// client headers are forwarded before channel-specific overrides are applied.
func TestSetupCommonRequestHeader(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{
		Header: make(http.Header),
	}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "application/json")
	c.Request.Header.Set("x-test-header", "test-value")

	req, _ := http.NewRequest("GET", "http://example.com", nil)

	m := &meta.Meta{
		IsStream: true,
	}

	SetupCommonRequestHeader(c, req, m)

	require.Equal(t, "application/json", req.Header.Get("Content-Type"))
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, "test-value", req.Header.Get("x-test-header"))
}

// TestApplyChannelCustomHeadersOverridesExistingHeaders verifies that
// channel-owned headers win over client-provided and provider-default headers.
func TestApplyChannelCustomHeadersOverridesExistingHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest("GET", "http://example.com", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer provider-default")
	req.Header.Set("X-Api-Key", "client-value")

	m := &meta.Meta{
		APIKey: "channel-secret",
		Config: model.ChannelConfig{
			CustomHeaders: map[string]string{
				"api-key":       channelAPIKeyPlaceholder,
				"Authorization": "Token " + channelAPIKeyPlaceholder,
				"X-Api-Key":     "channel-value",
			},
		},
	}

	err = applyChannelCustomHeaders(req, m)
	require.NoError(t, err)
	require.Equal(t, "channel-secret", req.Header.Get("api-key"))
	require.Equal(t, "Token channel-secret", req.Header.Get("Authorization"))
	require.Equal(t, "channel-value", req.Header.Get("X-Api-Key"))
}

// TestApplyChannelCustomHeadersRejectsInvalidNames verifies malformed custom
// header keys fail before an upstream request is sent.
func TestApplyChannelCustomHeadersRejectsInvalidNames(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest("GET", "http://example.com", nil)
	require.NoError(t, err)

	m := &meta.Meta{
		Config: model.ChannelConfig{
			CustomHeaders: map[string]string{
				"bad\nname": "value",
			},
		},
	}

	err = applyChannelCustomHeaders(req, m)
	require.Error(t, err)
}
