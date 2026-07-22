// Package state implements the gateway-owned Responses state layer: canonical
// response and conversation records, a lossless item ledger, provider affinity,
// and a pluggable encrypted store. It is the resolution layer that runs before
// route selection and format conversion, as described in
// docs/proposals/20260719_stateful-responses-format-conversion.md.
//
// The layer is deliberately independent of the Chat Completions, Responses, and
// Claude Messages converters: it produces a fully resolved turn, and exactly one
// converter then lowers that turn to a single upstream format.
package state

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Gateway ID prefixes. They intentionally match the OpenAI wire shape so client
// SDKs keep working unchanged; a gateway ID is distinguished from a raw provider
// ID by store lookup, never by prefix parsing.
const (
	responseIDPrefix     = "resp_"
	conversationIDPrefix = "conv_"
	itemIDPrefix         = "item_"

	// gatewayIDEntropyBytes yields 128 bits of randomness (32 hex chars), the
	// minimum entropy mandated by the proposal (SEC05).
	gatewayIDEntropyBytes = 16
)

// randomHex returns 2*n hex characters sourced from crypto/rand.
func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.Wrap(err, "read crypto/rand entropy")
	}
	return hex.EncodeToString(buf), nil
}

// NewResponseID returns a fresh gateway response ID: resp_<32 hex>.
func NewResponseID() (string, error) {
	suffix, err := randomHex(gatewayIDEntropyBytes)
	if err != nil {
		return "", err
	}
	return responseIDPrefix + suffix, nil
}

// NewConversationID returns a fresh gateway conversation ID: conv_<32 hex>.
func NewConversationID() (string, error) {
	suffix, err := randomHex(gatewayIDEntropyBytes)
	if err != nil {
		return "", err
	}
	return conversationIDPrefix + suffix, nil
}

// NewItemID returns a fresh gateway item ID: item_<32 hex>.
func NewItemID() (string, error) {
	suffix, err := randomHex(gatewayIDEntropyBytes)
	if err != nil {
		return "", err
	}
	return itemIDPrefix + suffix, nil
}

// LooksLikeGatewayResponseID reports whether id has the gateway response prefix
// and the expected width. It is a cheap shape check only; authority to resolve an
// ID always comes from a store lookup, never from this function.
func LooksLikeGatewayResponseID(id string) bool {
	return hasGatewayShape(id, responseIDPrefix)
}

// LooksLikeGatewayConversationID reports whether id has the gateway conversation
// prefix and the expected width.
func LooksLikeGatewayConversationID(id string) bool {
	return hasGatewayShape(id, conversationIDPrefix)
}

// LooksLikeGatewayItemID reports whether id has the gateway item prefix and the
// expected width.
func LooksLikeGatewayItemID(id string) bool {
	return hasGatewayShape(id, itemIDPrefix)
}

// LooksLikeLegacySyntheticID reports whether id is a legacy hyphenated synthetic
// fallback ID (resp-<request-id>) produced before the gateway state layer. These
// IDs are never resolvable and, once the feature is enabled, must return the
// standard not-found error rather than being forwarded upstream.
func LooksLikeLegacySyntheticID(id string) bool {
	return strings.HasPrefix(id, "resp-")
}

func hasGatewayShape(id, prefix string) bool {
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	suffix := id[len(prefix):]
	if len(suffix) != gatewayIDEntropyBytes*2 {
		return false
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
