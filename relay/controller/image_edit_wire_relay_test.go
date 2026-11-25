package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// imageEditWirePart is one multipart part a provider received.
type imageEditWirePart struct {
	name, filename string
	content        []byte
}

// readImageEditWireParts parses the multipart body a provider received.
// Parameters: t owns the assertions, contentType is the provider request's
// Content-Type, and body its body. Returns: the parts in order.
func readImageEditWireParts(t *testing.T, contentType string, body []byte) []imageEditWirePart {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType, "the provider must receive a multipart edit")
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var parts []imageEditWirePart
	for {
		part, err := reader.NextPart()
		if err == io.EOF { //nolint:errorlint // the multipart reader returns a bare io.EOF at the end
			return parts
		}
		require.NoError(t, err, "the provider body must be valid under the forwarded boundary")
		content, err := io.ReadAll(part)
		require.NoError(t, err)
		parts = append(parts, imageEditWirePart{name: part.FormName(), filename: part.FileName(), content: content})
	}
}

// TestImageEditWireKeepsReferenceImages drives image edits that the relay must
// rewrite (Azure always; any channel whose model mapping renames the model)
// and checks the bytes the provider receives: every reference image and the
// mask arrive byte-for-byte under a valid Content-Type, the model is the
// mapped provider model, Azure still omits the controls it does not document,
// and fields the typed request does not model survive. Before the fix these
// edits were re-encoded from the typed request, which has no image, mask, or
// edit-only fields, so the provider received an edit without its references
// (a multipart edit even arrived as JSON under a multipart Content-Type).
// Parameters: t runs the cases. Returns: none.
func TestImageEditWireKeepsReferenceImages(t *testing.T) {
	const balance = int64(10_000_000)
	const priced = `{"gpt-image-2.5-sunburst":{"image":{"price_per_image_usd":0.2}}}`
	refA := encodeTestImage(t, "png", 96, 64)
	refB := encodeTestImage(t, "png", 64, 96)
	mask := encodeTestImage(t, "png", 96, 64)
	multipartEdit := func(t *testing.T, model string) ([]byte, string) {
		body, boundary := buildTestMultipart(t,
			testMultipartPart{name: "model", content: []byte(model)},
			testMultipartPart{name: "prompt", content: []byte("A lighthouse")},
			testMultipartPart{name: "n", content: []byte("2")},
			testMultipartPart{name: "input_fidelity", content: []byte("high")},
			testMultipartPart{name: "safety_identifier", content: []byte("user-hash")},
			testMultipartPart{name: "image[]", filename: "a.png", content: refA},
			testMultipartPart{name: "image[]", filename: "b.png", content: refB},
			testMultipartPart{name: "mask", filename: "mask.png", content: mask},
		)
		return body, "multipart/form-data; boundary=" + boundary
	}

	for _, tc := range []struct {
		name, clientModel string
		channelType       int
		omitsSafety       bool
	}{
		{name: "azure_multipart_edit", clientModel: "gpt-image-2.5-sunburst", channelType: channeltype.Azure, omitsSafety: true},
		{name: "mapped_multipart_edit", clientModel: "house-image-alias", channelType: channeltype.OpenAI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, contentType := multipartEdit(t, tc.clientModel)
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: "image-edit-wire-" + tc.name, channelType: tc.channelType, model: "gpt-image-2.5-sunburst",
				path: "/v1/images/edits", contentType: contentType, body: body, configs: priced, usage: `null`, balance: balance,
			})
			// Two images at the 0.2 USD fallback: the client's own n is held.
			requireStrictImageSettled(t, res, balance, 200_000, 200_000, "configured_fallback")
			require.Contains(t, res.sentURL, "/images/edits")
			parts := readImageEditWireParts(t, res.sentContentType, res.sentBody)
			byName := map[string][]imageEditWirePart{}
			for _, part := range parts {
				byName[part.name] = append(byName[part.name], part)
			}
			require.Len(t, byName["model"], 1)
			require.Equal(t, "gpt-image-2.5-sunburst", string(byName["model"][0].content), "the provider renders the model billing priced")
			require.Len(t, byName["image[]"], 2, "every reference image reaches the provider")
			require.Equal(t, refA, byName["image[]"][0].content)
			require.Equal(t, "a.png", byName["image[]"][0].filename)
			require.Equal(t, refB, byName["image[]"][1].content)
			require.Len(t, byName["mask"], 1, "the mask reaches the provider")
			require.Equal(t, mask, byName["mask"][0].content)
			require.Equal(t, "high", string(byName["input_fidelity"][0].content), "untyped edit controls survive")
			require.Equal(t, "2", string(byName["n"][0].content))
			if tc.omitsSafety {
				require.NotContains(t, byName, "safety_identifier", "Azure edit bodies keep omitting undocumented controls")
			} else {
				require.Equal(t, "user-hash", string(byName["safety_identifier"][0].content))
			}
		})
	}

	jsonEdit := func(model string) []byte {
		return []byte(`{"model":"` + model + `","prompt":"A lighthouse","n":1,"safety_identifier":"user-hash",` +
			`"images":[{"image_url":"https://example.localhost/a.png"},{"file_id":"file-123"}],` +
			`"mask":{"file_id":"file-mask"},"output_compression":70}`)
	}
	for _, tc := range []struct {
		name, clientModel string
		channelType       int
		omitsSafety       bool
	}{
		{name: "azure_json_edit", clientModel: "gpt-image-2.5-sunburst", channelType: channeltype.Azure, omitsSafety: true},
		{name: "mapped_json_edit", clientModel: "house-image-alias", channelType: channeltype.OpenAI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: "image-edit-wire-" + tc.name, channelType: tc.channelType, model: "gpt-image-2.5-sunburst",
				path: "/v1/images/edits", contentType: "application/json", body: jsonEdit(tc.clientModel),
				configs: priced, usage: `null`, balance: balance,
			})
			requireStrictImageSettled(t, res, balance, 100_000, 100_000, "configured_fallback")
			require.True(t, strings.HasPrefix(res.sentContentType, "application/json"), "content-type=%s", res.sentContentType)
			var sent map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(res.sentBody, &sent), "body=%s", res.sentBody)
			require.JSONEq(t, `"gpt-image-2.5-sunburst"`, string(sent["model"]))
			require.JSONEq(t, `[{"image_url":"https://example.localhost/a.png"},{"file_id":"file-123"}]`, string(sent["images"]),
				"every reference image reaches the provider")
			require.JSONEq(t, `{"file_id":"file-mask"}`, string(sent["mask"]))
			require.JSONEq(t, `70`, string(sent["output_compression"]), "untyped edit controls survive")
			if tc.omitsSafety {
				require.NotContains(t, sent, "safety_identifier", "Azure edit bodies keep omitting undocumented controls")
			} else {
				require.JSONEq(t, `"user-hash"`, string(sent["safety_identifier"]))
			}
		})
	}
}
