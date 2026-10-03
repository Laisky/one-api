package relay_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
)

// catalogSnapshot records only compiled public defaults, never channel settings,
// credentials, account-specific model discovery, or live provider responses.
type catalogSnapshot struct {
	APIType        int                            `json:"api_type"`
	ChannelType    int                            `json:"channel_type"`
	Implementation string                         `json:"implementation"`
	Models         []string                       `json:"models"`
	Pricing        map[string]adaptor.ModelConfig `json:"pricing"`
}

// TestCompiledAdaptorCatalogSnapshot checks every registered adaptor and generic
// channel's assembled discovery list with t and records reproducible defaults in
// the existing Go JSON test artifacts. It returns nothing and uses no network.
func TestCompiledAdaptorCatalogSnapshot(t *testing.T) {
	// Run serially because other relay tests can temporarily replace globals.
	for apiType := 0; apiType < apitype.Dummy; apiType++ {
		t.Run(fmt.Sprintf("api-%02d", apiType), func(t *testing.T) {
			a := relay.GetAdaptor(apiType)
			require.NotNil(t, a, "registered API type must resolve")
			recordCatalogSnapshot(t, a, apiType, 0)
		})
	}
	// Several provider directories are selected by the generic OpenAI adaptor,
	// not by a distinct GetAdaptor case. Inspect those channel-specific tables too.
	for channel := 1; channel < channeltype.Dummy; channel++ {
		if channeltype.ToAPIType(channel) != apitype.OpenAI {
			continue
		}
		t.Run(fmt.Sprintf("channel-%02d", channel), func(t *testing.T) {
			recordCatalogSnapshot(t, &openai.Adaptor{ChannelType: channel}, apitype.OpenAI, channel)
		})
	}
}

// recordCatalogSnapshot checks a's public defaults for apiType/channel with t,
// then logs a gzip/base64 snapshot in bounded numbered chunks. It returns nothing.
// The data can be decoded from the existing go-tests-packages JSON artifact.
func recordCatalogSnapshot(t *testing.T, a adaptor.Adaptor, apiType, channel int) {
	t.Helper()
	snapshot := catalogSnapshot{
		APIType: apiType, ChannelType: channel,
		Implementation: fmt.Sprintf("%T", a),
		Models:         a.GetModelList(), Pricing: a.GetDefaultModelPricing(),
	}
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err, "defaults must be JSON-serializable (including finite prices)")
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(encoded)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	payload := base64.StdEncoding.EncodeToString(compressed.Bytes())
	const chunkSize = 4096
	chunks := (len(payload) + chunkSize - 1) / chunkSize
	for offset := 0; offset < len(payload); offset += chunkSize {
		end := min(offset+chunkSize, len(payload))
		t.Logf("CATALOG_V1 %d %d %d/%d %s", apiType, channel, offset/chunkSize+1, chunks, payload[offset:end])
	}
	seen := make(map[string]bool, len(snapshot.Models))
	for _, id := range snapshot.Models {
		require.NotEmpty(t, id)
		require.Equal(t, strings.TrimSpace(id), id, "model IDs must not contain surrounding whitespace")
		require.False(t, seen[id], "duplicate discoverable model %q", id)
		seen[id] = true
	}
}
