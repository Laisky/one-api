package groq

import (
	"bytes"
	"io"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// prepareGroqRequestBody applies Groq's Chat contract after all protocol and
// passthrough merges, including Claude conversion and Responses fallback.
// It returns a replayable reader or a wrapped read error. Non-text request
// modes retain their original readers, avoiding multipart/audio buffering.
func prepareGroqRequestBody(info *meta.Meta, body io.Reader) (io.Reader, error) {
	if body == nil || info == nil {
		return body, nil
	}
	switch info.Mode {
	case relaymode.ChatCompletions, relaymode.ClaudeMessages, relaymode.ResponseAPI:
	default:
		return body, nil
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read Groq request body for parameter compatibility")
	}
	normalized, _ := normalizeGroqChatParameters(raw)
	return bytes.NewReader(normalized), nil
}
