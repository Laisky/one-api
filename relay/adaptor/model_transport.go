package adaptor

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// validateModelTransport rejects catalog-only models before a Google REST request
// can prepare credentials or reach the network. Parameters: m carries the selected
// channel and mapped upstream model. Returns: a Live API requirement error or nil.
func validateModelTransport(m *meta.Meta) error {
	if m == nil {
		return nil
	}
	switch m.ChannelType {
	case channeltype.Gemini, channeltype.VertextAI, channeltype.GeminiOpenAICompatible:
	default:
		// An unrelated provider may implement a REST bridge for the same ID.
		return nil
	}

	switch m.ActualModelName {
	case "gemini-3.8-live", "gemini-3.8-live-extended-thinking":
		return errors.Errorf("model %q requires the Gemini Live API and cannot be dispatched through this REST adaptor", m.ActualModelName)
	default:
		return nil
	}
}
