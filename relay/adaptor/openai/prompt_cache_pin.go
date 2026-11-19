package openai

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// promptCacheExplicitPinKey stores the admission decision that the current
// attempt must disable OpenAI's implicit cache breakpoint before dispatch.
const promptCacheExplicitPinKey = "openai_prompt_cache_explicit_pin"

// promptCacheExplicitPin binds the admission decision to one channel and mapped
// model, so a retry on another channel can never inherit it.
type promptCacheExplicitPin struct {
	channelID int
	model     string
}

// SetPromptCacheExplicitPin records whether the admitted attempt must be sent
// with prompt_cache_options.mode="explicit". Admission sets it when the request
// reaches a catalog tier whose cache-write price is unpublished and the client
// did not ask for cache writes, so OpenAI's implicit caching cannot create an
// unpriced write. Every admission records its decision, clearing a previous
// attempt's. Parameters: c is the request context, m identifies the attempt
// and pin is the decision. It returns nothing.
func SetPromptCacheExplicitPin(c *gin.Context, m *meta.Meta, pin bool) {
	if c == nil {
		return
	}
	if !pin || m == nil {
		c.Set(promptCacheExplicitPinKey, nil)
		return
	}
	c.Set(promptCacheExplicitPinKey, &promptCacheExplicitPin{channelID: m.ChannelId, model: m.ActualModelName})
}

// promptCacheExplicitPinned reports whether admission pinned explicit cache
// mode for exactly this native OpenAI attempt. Parameters: c is the request
// context and m the attempt metadata. It returns false for other channels,
// models, or attempts.
func promptCacheExplicitPinned(c *gin.Context, m *meta.Meta) bool {
	if c == nil || m == nil || m.ChannelType != channeltype.OpenAI {
		return false
	}
	stored, exists := c.Get(promptCacheExplicitPinKey)
	if !exists {
		return false
	}
	pin, ok := stored.(*promptCacheExplicitPin)
	return ok && pin != nil && pin.channelID == m.ChannelId && pin.model == m.ActualModelName
}

// applyPromptCacheExplicitPin rewrites a pinned attempt's JSON body so OpenAI
// creates no implicit cache breakpoint. Admission already refused explicit
// client breakpoints for such requests, so explicit mode guarantees the
// receipt has no cache write in the unpriced tier. Parameters: c and m identify
// the attempt and body is the prepared provider payload. It returns the body to
// dispatch, unchanged when no pin applies, or an error when a pinned body
// cannot be rewritten (dispatching it could incur an unpriced write).
func applyPromptCacheExplicitPin(c *gin.Context, m *meta.Meta, body io.Reader) (io.Reader, error) {
	if body == nil || !promptCacheExplicitPinned(c, m) {
		return body, nil
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read pinned prompt-cache request body")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return nil, errors.New("pinned prompt-cache request body must be a JSON object")
	}
	options := map[string]json.RawMessage{}
	if existing, ok := root[promptCacheOptionsKey]; ok && !isJSONNull(existing) {
		if err := json.Unmarshal(existing, &options); err != nil || options == nil {
			return nil, errors.New("pinned prompt_cache_options must be an object")
		}
	}
	options["mode"] = json.RawMessage(`"` + promptCacheModeExplicit + `"`)
	encodedOptions, err := json.Marshal(options)
	if err != nil {
		return nil, errors.Wrap(err, "encode pinned prompt_cache_options")
	}
	root[promptCacheOptionsKey] = encodedOptions
	patched, err := json.Marshal(root)
	if err != nil {
		return nil, errors.Wrap(err, "encode pinned prompt-cache request body")
	}
	gmw.GetLogger(c).Debug("disabled implicit prompt caching for a tier without a published cache-write price",
		zap.String("model", m.ActualModelName),
		zap.Int("prompt_tokens", m.PromptTokens))
	return bytes.NewReader(patched), nil
}
