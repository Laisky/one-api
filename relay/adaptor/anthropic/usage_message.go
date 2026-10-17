package anthropic

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// ParseClaudeMessageUsage extracts bounded native message billing evidence.
// Parameters: body is one upstream JSON message. Returns: the shared validated
// token receipt, with the existing estimate label for missing counters, or an
// error for malformed/negative/overflowing evidence. It does not record tools.
func ParseClaudeMessageUsage(body []byte) (*model.Usage, error) {
	if len(body) > maxClaudeHTTPPayload {
		return nil, errors.New("Claude response exceeds gateway 32 MiB limit")
	}
	var message struct {
		Type  string          `json:"type"`
		Usage json.RawMessage `json:"usage"`
		Error Error           `json:"error"`
	}
	if err := decodeClaudeJSON(body, &message); err != nil {
		return nil, errors.Wrap(err, "decode Claude usage message")
	}
	if message.Type != "message" || message.Error.Type != "" {
		return nil, errors.New("Claude usage evidence is not a successful message")
	}
	var receipt httpUsageReceipt
	if err := receipt.apply(message.Usage); err != nil {
		return nil, err
	}
	return retainedHTTPUsage(&receipt), nil
}
