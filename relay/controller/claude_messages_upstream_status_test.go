package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestClaudeUpstreamFailed pins the Claude controller's provider status rule:
// only 200, a missing SDK response, and Replicate's 201 prediction
// acknowledgement are successes.
func TestClaudeUpstreamFailed(t *testing.T) {
	t.Parallel()
	replicate := &metalib.Meta{ChannelType: channeltype.Replicate}
	openAI := &metalib.Meta{ChannelType: channeltype.OpenAI}
	for _, tc := range []struct {
		name   string
		meta   *metalib.Meta
		resp   *http.Response
		failed bool
	}{
		{name: "sdk transport without response", meta: openAI, resp: nil, failed: false},
		{name: "ok", meta: openAI, resp: &http.Response{StatusCode: http.StatusOK}, failed: false},
		{name: "replicate prediction created", meta: replicate, resp: &http.Response{StatusCode: http.StatusCreated}, failed: false},
		{name: "created from another provider", meta: openAI, resp: &http.Response{StatusCode: http.StatusCreated}, failed: true},
		{name: "created without meta", meta: nil, resp: &http.Response{StatusCode: http.StatusCreated}, failed: true},
		{name: "replicate rejection", meta: replicate, resp: &http.Response{StatusCode: http.StatusUnprocessableEntity}, failed: true},
		{name: "server error", meta: openAI, resp: &http.Response{StatusCode: http.StatusInternalServerError}, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.failed, claudeUpstreamFailed(tc.meta, tc.resp))
		})
	}
}
