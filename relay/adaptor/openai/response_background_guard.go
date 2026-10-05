package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// ErrResponseBackgroundUnsupported marks Responses background execution. One-API
// rejects it before reservation or dispatch until a durable asynchronous
// settlement lifecycle exists, because a queued job's initial reply carries no
// authoritative terminal usage (issue #483).
var ErrResponseBackgroundUnsupported = errors.New("background responses are unavailable until durable terminal billing is supported")

// responseBackgroundField is the canonical Responses request field name.
const responseBackgroundField = "background"

// nonTerminalResponseEstimateReason labels settlements that retained the whole
// reservation because the provider answered with unfinished work.
const nonTerminalResponseEstimateReason = "response_nonterminal_status"

// IsResponseBackgroundKey reports whether key would be decoded as the Responses
// "background" field by a case-insensitive JSON decoder. The parameter key is a
// decoded (unescaped) JSON object key. It returns true for any spelling that
// matches under Unicode simple case folding, which is what encoding/json uses,
// and additionally ignores '_' and '-' so looser name matchers are covered.
func IsResponseBackgroundKey(key string) bool {
	folded := strings.Map(func(r rune) rune {
		if r == '_' || r == '-' {
			return -1
		}
		return r
	}, key)
	return strings.EqualFold(folded, responseBackgroundField)
}

// ValidateResponseBackgroundPayload checks the root object of a Responses
// request or WebSocket response.create frame. The parameter payload is the raw
// client JSON. It returns an error wrapping ErrResponseBackgroundUnsupported when
// any root key that folds to "background" carries a value other than false or
// null, or when more than one such key is present, because decoders disagree on
// which duplicate wins. It returns nil for payloads that are not JSON objects or
// that stop parsing before any background key: every forwarder re-decodes the
// payload and never sends raw bytes that fail to parse as an object.
func ValidateResponseBackgroundPayload(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	opening, err := decoder.Token()
	if err != nil {
		return nil // Malformed input is rejected by the caller's own decoder.
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return nil
	}

	seen := 0
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil // Malformed input is rejected by the caller's own decoder.
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil // Malformed input is rejected by the caller's own decoder.
		}
		if !IsResponseBackgroundKey(key) {
			continue
		}
		seen++
		if seen > 1 {
			return errors.Wrap(ErrResponseBackgroundUnsupported, "ambiguous duplicate background keys")
		}
		switch string(bytes.TrimSpace(value)) {
		case "false", "null":
		default:
			return errors.WithStack(ErrResponseBackgroundUnsupported)
		}
	}
	return nil
}

// StripResponseBackgroundKeys deletes every root key of an outgoing Responses
// payload that folds to "background", so a provider can never receive a
// background flag from one-api. The parameter root is the decoded root object
// and is modified in place. It returns true when at least one key was removed.
func StripResponseBackgroundKeys[V any](root map[string]V) bool {
	removed := false
	for key := range root {
		if IsResponseBackgroundKey(key) {
			delete(root, key)
			removed = true
		}
	}
	return removed
}

// IsResponseStatusNonTerminal reports whether a Responses status names work the
// provider accepted but has not finished. The parameter status is the provider
// "status" value. It returns true for "queued" and "in_progress", whose replies
// carry no authoritative final usage and must not be settled or continued.
func IsResponseStatusNonTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "in_progress":
		return true
	default:
		return false
	}
}

// nonTerminalResponseUsage builds the settlement usage for an unexpected
// non-terminal, non-streaming Responses reply. Parameters: c is the request
// context used for logging and response is the decoded provider reply. It
// returns any provider-reported counters (never synthesized ones) marked as an
// estimate, so post-billing retains at least the full reservation and never
// refunds work that may still complete upstream.
func nonTerminalResponseUsage(c *gin.Context, response *ResponseAPIResponse) *model.Usage {
	lg := gmw.GetLogger(c)
	usage := &model.Usage{}
	if response.Usage != nil {
		if converted := response.Usage.ToModelUsage(); converted != nil {
			usage = converted
		}
	}
	usage.BillingEstimateReason = nonTerminalResponseEstimateReason
	lg.Warn("provider returned a non-terminal Responses reply; retaining the full reservation",
		zap.String("status", response.Status),
		zap.String("response_id", response.Id),
		zap.Int("reported_prompt_tokens", usage.PromptTokens),
		zap.Int("reported_completion_tokens", usage.CompletionTokens),
	)
	return usage
}
