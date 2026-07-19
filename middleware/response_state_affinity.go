package middleware

import (
	"encoding/json"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/state"
)

// responseStateSelectorProbe is the minimal shape needed to detect the gateway
// state selectors on a /v1/responses request without decoding the full body.
type responseStateSelectorProbe struct {
	PreviousResponseId *string         `json:"previous_response_id"`
	Conversation       json.RawMessage `json:"conversation"`
}

// parseResponseStateSelectors extracts the previous_response_id and conversation
// selectors from the (reusable) request body. Both are optional; the body has
// already been buffered by TokenAuth's model extraction so this does not consume
// it. Parsing is fully defensive: a missing or malformed body yields empty
// selectors and never fails the request.
func parseResponseStateSelectors(c *gin.Context) (prevID, convID string) {
	var probe responseStateSelectorProbe
	if err := common.UnmarshalBodyReusable(c, &probe); err != nil {
		return "", ""
	}
	if probe.PreviousResponseId != nil {
		prevID = strings.TrimSpace(*probe.PreviousResponseId)
	}
	convID = parseConversationSelector(probe.Conversation)
	return prevID, convID
}

// parseConversationSelector accepts both the string form ("conv_...") and the
// object form ({"id":"conv_..."}) of the conversation selector.
func parseConversationSelector(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asObject struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return strings.TrimSpace(asObject.Id)
	}
	return ""
}

// responseStateAffinityChannel resolves the channel bound to a referenced gateway
// response or conversation (provider affinity, proposal ST-005 / row R01). It
// returns a channel only when the binding is present AND the bound channel is
// still eligible for this request (enabled, serves the group, supports the model,
// endpoint, and — for websocket handshakes — the Responses websocket transport,
// row R07). Any miss, unhealthy binding, or store error returns nil so the caller
// falls back to normal selection and the fallback path replays canonical items
// (rows R02, R05). It is a no-op unless the feature is enabled for this owner.
func responseStateAffinityChannel(c *gin.Context, relayMode int, userGroup, requestModel string, isResponseWSHandshake bool) *model.Channel {
	if relayMode != relaymode.ResponseAPI {
		return nil
	}
	if !state.Enabled() || state.Store() == nil {
		return nil
	}
	userID := c.GetInt(ctxkey.Id)
	tokenID := c.GetInt(ctxkey.TokenId)
	owner := state.OwnerScope{UserID: userID, TokenID: tokenID}
	if !owner.Valid() {
		return nil
	}
	// The channel-scoped allowlist check is deferred until the bound channel is
	// known (below), so a channel-only allowlist still enables affinity for its
	// channels — a channel is unknown at this pre-routing point.

	prevID, convID := parseResponseStateSelectors(c)
	if prevID == "" && convID == "" {
		return nil
	}

	ctx := gmw.Ctx(c)
	var binding *state.ProviderBinding
	switch {
	case prevID != "" && state.LooksLikeGatewayResponseID(prevID):
		b, err := state.Store().GetResponseBinding(ctx, owner, prevID)
		if err != nil {
			// Fail open to normal selection; the hydration path performs the
			// authoritative lookup and surfaces the typed error if one is due.
			return nil
		}
		binding = b
	case convID != "" && state.LooksLikeGatewayConversationID(convID):
		conv, err := state.Store().GetConversation(ctx, owner, convID)
		if err != nil {
			return nil
		}
		binding = conv.Binding
	}
	if binding == nil || binding.ChannelID == 0 {
		return nil
	}

	channel, err := model.GetChannelById(binding.ChannelID, true)
	if err != nil || channel == nil {
		return nil
	}
	// Now that the bound channel is known, apply the owner/channel allowlist. This
	// honors user, token, and channel-scoped allowlists (row O03).
	if !state.AllowedFor(userID, tokenID, channel.Id) {
		return nil
	}
	if channel.Status != model.ChannelStatusEnabled {
		return nil
	}
	if !channelSupportsGroup(channel, userGroup) {
		return nil
	}
	if requestModel != "" && !channel.SupportsModel(requestModel) {
		return nil
	}
	if !channelSupportsEndpoint(channel, relayMode) {
		return nil
	}
	if !channelSupportsResponseWebSocket(channel, relayMode, isResponseWSHandshake) {
		return nil
	}

	metrics.RecordStateEvent(metrics.StateCategoryAffinity, metrics.StateOutcomePinned)
	gmw.GetLogger(c).Debug("response state affinity pinned bound channel",
		zap.Int("channel_id", channel.Id),
		zap.Int("user_id", userID),
	)
	return channel
}
