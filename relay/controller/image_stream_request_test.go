package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestBuildOpenAIImageRequestKeepsStreamOptions checks that a re-encoded
// OpenAI or Azure generation keeps a GPT Image client's stream and
// partial_images, so a streaming client receives the event stream it asked
// for and the partial images it is billed for, while legacy DALL-E bodies,
// which cannot stream, never carry them. Parameters: t runs the cases.
// Returns: none.
func TestBuildOpenAIImageRequestKeepsStreamOptions(t *testing.T) {
	for _, tc := range []struct {
		name, model, want string
		channelType       int
	}{
		{name: "openai_gpt_image", model: "gpt-image-2.5-sunburst", channelType: channeltype.OpenAI,
			want: `{"model":"gpt-image-2.5-sunburst","prompt":"p","n":1,"stream":true,"partial_images":2}`},
		{name: "azure_gpt_image", model: "gpt-image-2", channelType: channeltype.Azure,
			want: `{"model":"gpt-image-2","prompt":"p","n":1,"stream":true,"partial_images":2}`},
		{name: "dalle_cannot_stream", model: "dall-e-3", channelType: channeltype.OpenAI,
			want: `{"model":"dall-e-3","prompt":"p","n":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &relaymodel.ImageRequest{Model: tc.model, Prompt: "p", N: 1, Stream: true, PartialImages: new(2)}
			encoded, err := json.Marshal(buildOpenAIImageRequestForChannel(req, tc.channelType))
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(encoded))
		})
	}
}

// TestRequestedPartialImages checks the partial image count the hold and the
// streamed settlement use: omitted is zero and values outside OpenAI's
// documented 0 to 3 are clamped. Parameters: t runs the cases. Returns: none.
func TestRequestedPartialImages(t *testing.T) {
	require.Zero(t, requestedPartialImages(nil))
	require.Zero(t, requestedPartialImages(&relaymodel.ImageRequest{}))
	for value, want := range map[int]int{-1: 0, 0: 0, 2: 2, 3: 3, 9: 3} {
		require.Equal(t, want, requestedPartialImages(&relaymodel.ImageRequest{PartialImages: new(value)}), "partial_images=%d", value)
	}
}
