package adaptor

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// ErrModelRequiresLiveTransport marks a request for a model Google publishes
// with `bidiGenerateContent` as its only generation method. It is a caller
// mistake, not a channel fault: the same request fails on every Google channel.
var ErrModelRequiresLiveTransport = errors.New("model requires the Gemini Live API")

// liveOnlyGoogleModels enumerates the catalog models whose only advertised
// generation method is bidiGenerateContent. Verified against Google's own model
// listing on 2026-09-18, where REST generateContent answers each of them with
// HTTP 400 "only supports real-time bidirectional streaming via WebSocket".
// This is the transport fact; realtime.IsGeminiLiveModel is the narrower set
// whose wire and billing contracts this gateway implements.
var liveOnlyGoogleModels = map[string]struct{}{
	"gemini-3.8-live":                   {},
	"gemini-3.8-live-extended-thinking": {},
	"gemini-3.1-flash-live-preview":     {},
	"gemini-3.5-live-translate-preview": {},
	"gemini-3.5-transcribe-live":        {},
}

// IsLiveOnlyGoogleModel reports whether Google serves a model exclusively over
// the Live WebSocket API. Parameters: model is the upstream ID. Returns: true
// for a model no REST endpoint can answer.
func IsLiveOnlyGoogleModel(model string) bool {
	_, live := liveOnlyGoogleModels[model]
	return live
}

// WithoutLiveOnlyGoogleModels drops the Live-only IDs from an advertised catalog.
// Parameters: models is a channel's candidate model list. Returns: a new slice
// for channel types that have no Live transport, so an operator filling a channel
// from it never publishes an ID that every transport rejects.
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

// ValidateModelTransport rejects catalog-only models before a Google REST request
// can prepare credentials or reach the network. Parameters: m carries the selected
// channel and mapped upstream model. Returns: a Live API requirement error or nil.
// Callers must surface it as a client error; answering 5xx would retry the same
// impossible request on further channels and count it against channel health.
func ValidateModelTransport(m *meta.Meta) error {
	if m == nil {
		return nil
	}
	switch m.ChannelType {
	case channeltype.Gemini, channeltype.VertextAI, channeltype.GeminiOpenAICompatible:
	default:
		// An unrelated provider may implement a REST bridge for the same ID.
		return nil
	}

	if _, live := liveOnlyGoogleModels[m.ActualModelName]; !live {
		return nil
	}
	return errors.Wrapf(ErrModelRequiresLiveTransport,
		"model %q cannot be dispatched through this REST adaptor; connect to GET /v1/realtime?model=%s instead",
		m.ActualModelName, m.ActualModelName)
}
