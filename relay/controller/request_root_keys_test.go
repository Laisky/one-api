package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// jsonUnicodeEscaped spells every rune of name as a six-character JSON
// unicode escape (backslash, 'u', four hex digits).
func jsonUnicodeEscaped(name string) string {
	var b strings.Builder
	for _, r := range name {
		fmt.Fprintf(&b, "%cu%04x", '\\', r)
	}
	return b.String()
}

// TestChatRawPassthroughSafeUsesDecodedKeys classifies bodies by decoded root
// keys: escapes and case folds of typed fields or extra_body force
// normalization, while look-alike text inside values does not.
func TestChatRawPassthroughSafeUsesDecodedKeys(t *testing.T) {
	t.Parallel()
	for body, safe := range map[string]bool{
		`{"model":"m","messages":[]}`: true,
		`{"model":"m","messages":[{"role":"user","content":"\"extra_body\":{}"}]}`:    true,
		`{"model":"m","vendor":{"extra_body":{"n":2}},"Vendor_Flag":1}`:               true,
		`{"model":"m","extra_body":{}}`:                                               false,
		`{"model":"m","extra` + jsonUnicodeEscaped("_") + `body":{}}`:                 false,
		`{"model":"m","` + jsonUnicodeEscaped("extra_body") + `":{}}`:                 false,
		`{"model":"m","Extra_Body":{}}`:                                               false,
		`{"model":"m","Max_Tokens":5}`:                                                false,
		`{"model":"m","max_to` + jsonUnicodeEscaped(string(rune(0x212A))) + `ens":5}`: false,
		`{"model":"m","model":"m"}`:                                                   false,
		`{"model":"m","` + jsonUnicodeEscaped("m") + `odel":"m"}`:                     false,
		`model=m`:         false,
		`[{"model":"m"}]`: false,
	} {
		require.Equal(t, safe, chatRawPassthroughSafe([]byte(body)), body)
	}
}

// TestCanonicalizeExtraBodyKey renames one folded spelling and refuses two.
func TestCanonicalizeExtraBodyKey(t *testing.T) {
	t.Parallel()
	root := map[string]json.RawMessage{"Extra_Body": json.RawMessage(`{"top_k":1}`), "model": json.RawMessage(`"m"`)}
	require.NoError(t, canonicalizeExtraBodyKey(root))
	require.Equal(t, map[string]json.RawMessage{"extra_body": json.RawMessage(`{"top_k":1}`), "model": json.RawMessage(`"m"`)}, root)

	plain := map[string]json.RawMessage{"extra_body": json.RawMessage(`{}`)}
	require.NoError(t, canonicalizeExtraBodyKey(plain))
	require.Contains(t, plain, "extra_body")

	both := map[string]json.RawMessage{"extra_body": json.RawMessage(`{}`), "EXTRA_BODY": json.RawMessage(`{}`)}
	require.ErrorIs(t, canonicalizeExtraBodyKey(both), common.ErrAmbiguousJSONKey)
}

// TestMergeControlledPassthroughJSONCanonicalizesFoldedExtraBody applies the
// extra_body allowlist to a case-folded spelling instead of preserving it as
// an unknown root field, and never forwards folded copies of typed fields.
func TestMergeControlledPassthroughJSONCanonicalizesFoldedExtraBody(t *testing.T) {
	t.Parallel()
	for _, allowUnknown := range []bool{true, false} {
		original := []byte(`{"model":"cheap","Max_Tokens":64,"Extra_Body":{"model":"premium","n":8,"top_k":5},"vendor_flag":true}`)
		updated := []byte(`{"model":"cheap","max_tokens":64}`)

		merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, allowUnknown)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, 2, stats.ExtraBodyRejected)
		require.Equal(t, 1, stats.ExtraBodyMerged)

		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(merged, &root))
		require.Equal(t, `"cheap"`, string(root["model"]))
		require.Equal(t, `64`, string(root["max_tokens"]))
		require.Equal(t, `5`, string(root["top_k"]))
		for key := range root {
			require.False(t, isExtraBodyKey(key), key)
			require.False(t, isNonCanonicalChatFieldSpelling(key), key)
		}
		require.NotContains(t, root, "n")
		_, preserved := root["vendor_flag"]
		require.Equal(t, allowUnknown, preserved)
	}

	_, _, _, err := mergeControlledPassthroughJSON([]byte(`{"extra_body":{},"Extra_Body":{}}`), []byte(`{"model":"m"}`), true)
	require.ErrorIs(t, err, common.ErrAmbiguousJSONKey)
	require.True(t, shouldTreatConvertRequestErrorAsBadRequest(err))
}

// TestGetRequestBodyNormalizesEscapedExtraBody drives the OpenAI-compatible
// passthrough branch directly: an escaped extra_body is flattened under the
// allowlist exactly like the plain spelling, and plain bodies stay raw.
func TestGetRequestBodyNormalizesEscapedExtraBody(t *testing.T) {
	old := config.EnforceIncludeUsage
	config.EnforceIncludeUsage = false
	t.Cleanup(func() { config.EnforceIncludeUsage = old })
	for _, key := range []string{`"extra_body"`, `"extra` + jsonUnicodeEscaped("_") + `body"`, `"EXTRA_BODY"`} {
		raw := `{"model":"custom-model","messages":[{"role":"user","content":"hello"}],` + key + `:{"model":"premium","n":8,"top_k":5}}`
		var request relaymodel.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
		m := &meta.Meta{ChannelType: channeltype.OpenAICompatible, APIType: apitype.OpenAI, OriginModelName: "custom-model", ActualModelName: "custom-model"}

		body, err := getRequestBody(c, m, &request, &openrouter.Adaptor{}, false)
		require.NoError(t, err)
		encoded, err := io.ReadAll(body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"custom-model","messages":[{"role":"user","content":"hello"}],"top_k":5}`, string(encoded), key)
	}
}
