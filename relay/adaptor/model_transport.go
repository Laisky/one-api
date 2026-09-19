package adaptor

import (
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// ErrModelRequiresLiveTransport marks a local Google REST incompatibility.
// Another channel may bridge the same model over REST; this is neither an
// upstream entitlement decision nor evidence of channel health.
var ErrModelRequiresLiveTransport = errors.New("model requires the Gemini Live API")

// liveOnlyGoogleModels records known native-only transport facts. The original
// five IDs came from the 2026-09-18 Developer catalog; the additional Vertex IDs
// come from https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api.
// This is a REST guard, not an allowlist for native Live model admission.
var liveOnlyGoogleModels = map[string]struct{}{
	"gemini-3.8-live":                     {},
	"gemini-3.8-live-extended-thinking":   {},
	"gemini-3.1-flash-live-preview":       {},
	"gemini-3.5-live-translate-preview":   {},
	"gemini-3.5-transcribe-live":          {},
	"gemini-live-2.5-flash-native-audio":   {},
	"gemini-3.5-transcribe-live-preview":  {},
}

// IsLiveOnlyGoogleModel reports whether Google's native generation transport
// for this ID is Live-only. Parameters: model is the upstream ID. Returns: true
// for known Live-only IDs, without restricting third-party REST bridges.
func IsLiveOnlyGoogleModel(model string) bool {
	_, live := liveOnlyGoogleModels[model]
	return live
}

// WithoutLiveOnlyGoogleModels filters a specifically REST-scoped list.
// Parameters: models are candidate IDs. Returns: a fresh slice excluding known
// native-only IDs. Do not use this to hide administrator model suggestions.
func WithoutLiveOnlyGoogleModels(models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if IsLiveOnlyGoogleModel(model) {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered
}

// IsRESTTransportMismatch reports whether err is the local routing condition where
// a Google REST adaptor cannot serve a Live-only model. Parameters: err is a
// normalized or wrapped error. Returns: true when another channel may still provide
// a compatible REST bridge for the requested model.
func IsRESTTransportMismatch(err error) bool {
	return errors.Is(err, ErrModelRequiresLiveTransport)
}

// ValidateRESTModelTransport rejects known incompatible models before a Google
// REST request can prepare credentials or reach the network. Parameters: m carries
// the selected channel and mapped model. Returns: a Live requirement error or nil.
// Native realtime requests deliberately do not call this REST-only guard.
func ValidateRESTModelTransport(m *meta.Meta) error {
	if m == nil {
		return nil
	}
	switch m.ChannelType {
	case channeltype.Gemini, channeltype.VertextAI, channeltype.GeminiOpenAICompatible:
	default:
		return nil
	}
	if !IsLiveOnlyGoogleModel(m.ActualModelName) {
		return nil
	}
	requestedModel := strings.TrimSpace(m.OriginModelName)
	if requestedModel == "" {
		requestedModel = m.ActualModelName
	}
	return errors.Wrapf(ErrModelRequiresLiveTransport,
		"model %q cannot be dispatched through this REST adaptor; connect to GET /v1/realtime?model=%s instead",
		m.ActualModelName, url.QueryEscape(requestedModel))
}
