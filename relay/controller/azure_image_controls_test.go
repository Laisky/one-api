package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestAzureImageGenerationOmitsUndocumentedControls proves the exact Azure
// image-generation body never carries OpenAI-only controls that Azure does not
// document for images (safety_identifier, complexity). Before PR #624 the typed
// request could not carry them, so they were silently dropped; forwarding them
// now would turn previously working Azure requests into "unrecognized request
// argument" rejections. The legacy user field and output_format are kept, and
// a native OpenAI channel (the control) still receives safety_identifier.
func TestAzureImageGenerationOmitsUndocumentedControls(t *testing.T) {
	const body = `{"model":"gpt-image-2.5-sunburst","prompt":"A lighthouse","safety_identifier":"sid-7f3a","user":"legacy-user","output_format":"webp"}`
	for _, tc := range []struct {
		channel      int
		wantSafetyID bool
	}{
		{channel: channeltype.OpenAI, wantSafetyID: true},
		{channel: channeltype.Azure, wantSafetyID: false},
	} {
		t.Run(channeltype.IdToName(tc.channel), func(t *testing.T) {
			got := runImage26ControlRelay(t, tc.channel, "gpt-image-2.5-sunburst", "/v1/images/generations", "application/json", []byte(body))
			require.Nil(t, got.Err)
			require.EqualValues(t, 1, got.Calls)
			var sent map[string]any
			require.NoError(t, json.Unmarshal(got.Body, &sent))
			require.Equal(t, "legacy-user", sent["user"])
			require.Equal(t, "webp", sent["output_format"])
			require.NotContains(t, sent, "complexity")
			if tc.wantSafetyID {
				require.Equal(t, "sid-7f3a", sent["safety_identifier"], "control: native OpenAI documents safety_identifier")
				return
			}
			require.NotContains(t, sent, "safety_identifier", "Azure image generation does not document safety_identifier")
		})
	}
}

// TestAzureImageEditJSONOmitsUndocumentedControls covers the Azure image-edit
// JSON path, which re-encodes the shared request rather than the generation
// schema: safety_identifier and complexity must not reach Azure there either.
func TestAzureImageEditJSONOmitsUndocumentedControls(t *testing.T) {
	const body = `{"model":"gpt-image-2.5-sunburst","prompt":"Add a boat","images":[{"image_url":"https://example.com/a.png"}],"safety_identifier":"sid-7f3a","user":"legacy-user"}`
	got := runImage26ControlRelay(t, channeltype.Azure, "gpt-image-2.5-sunburst", "/v1/images/edits", "application/json", []byte(body))
	require.Nil(t, got.Err)
	require.EqualValues(t, 1, got.Calls)
	var sent map[string]any
	require.NoError(t, json.Unmarshal(got.Body, &sent), string(got.Body))
	require.Equal(t, "legacy-user", sent["user"])
	require.NotContains(t, sent, "safety_identifier")
	require.NotContains(t, sent, "complexity")
}
