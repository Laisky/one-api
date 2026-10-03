package replicate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetRequestURLRejectsImageModelsOutsideImageRelay verifies that the final
// upstream routing boundary rejects image-priced models even when conversion
// is bypassed by raw passthrough or a different inbound protocol.
func TestGetRequestURLRejectsImageModelsOutsideImageRelay(t *testing.T) {
	t.Parallel()
	for name, pricing := range ModelRatios {
		if pricing.Image == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			provider := &Adaptor{}
			url, err := provider.GetRequestURL(&meta.Meta{
				OriginModelName: name,
				Mode:            relaymode.ChatCompletions,
			})
			require.Error(t, err, "image models must not obtain a token-billed upstream URL")
			require.Empty(t, url)
			require.Contains(t, err.Error(), "image API")
		})
	}
}

// TestGetRequestURLPreservesImageAndChatRoutes verifies that enforcing the
// billing boundary does not disable correctly routed image or language models.
func TestGetRequestURLPreservesImageAndChatRoutes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mode int
	}{
		{"black-forest-labs/flux-pro", relaymode.ImagesGenerations},
		{"black-forest-labs/flux-pro", relaymode.ImagesEdits},
		{"anthropic/claude-3.5-haiku", relaymode.ChatCompletions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &Adaptor{}
			url, err := provider.GetRequestURL(&meta.Meta{OriginModelName: tc.name, Mode: tc.mode})
			require.NoError(t, err)
			require.Equal(t, "https://api.replicate.com/v1/models/"+tc.name+"/predictions", url)
		})
	}
}
