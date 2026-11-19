package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// MaxPromptCacheKeyLength is OpenAI's maximum prompt_cache_key length in
// characters. OpenAI answers longer keys with HTTP 400 ("string too long.
// Expected a string with maximum length 64"), so the gateway never forwards
// such a key: it is dropped, and the request is served without cache routing,
// exactly as every route did before the key was forwarded at all.
const MaxPromptCacheKeyLength = 64

// promptCacheKeyNamespaceDigestBytes is the number of SHA-256 bytes kept in a
// forwarded key: 128 bits, rendered as 32 hex characters, well inside
// MaxPromptCacheKeyLength while keeping tenant partitions collision-free.
const promptCacheKeyNamespaceDigestBytes = 16

// promptCacheKeyNamespaceDomain separates prompt_cache_key derivation from
// every other SHA-256 use and versions the derivation, so a future scheme
// change produces new partitions instead of colliding with old ones.
const promptCacheKeyNamespaceDomain = "one-api/prompt_cache_key/v1"

// ErrInvalidPromptCacheKey marks a malformed client prompt_cache_key.
var ErrInvalidPromptCacheKey = errors.New("invalid prompt_cache_key")

// PromptCacheKey carries an OpenAI prompt_cache_key through the gateway.
//
// Decoding stores the client's value, which is never serialized: one-api shares
// one upstream organisation across tenants, so forwarding a raw key would let
// two tenants that pick the same key share a prompt-cache partition (and probe
// each other's cache hits). Encoding emits only the forwarded value set by
// ScopeToUser, a deterministic opaque derivation of the gateway user and the
// client key. The zero value, and any key that was not scoped, is omitted from
// JSON through the `omitzero` tag option, so destinations that never scope the
// key never receive the field.
type PromptCacheKey struct {
	client    string
	forwarded string
	// invalid marks a non-string or oversize client value. Decoding never
	// fails on it (a malformed optional routing hint must not break requests
	// that never forwarded it); the key is simply never forwarded.
	invalid bool
}

// NewPromptCacheKey returns a PromptCacheKey holding client as the client's
// value. Parameters: client is the client-supplied key. It returns an
// unscoped key; callers validate length through UnmarshalJSON or Validate.
func NewPromptCacheKey(client string) PromptCacheKey {
	return PromptCacheKey{client: client}
}

// UnmarshalJSON decodes a client prompt_cache_key. A malformed value (a
// non-string, or a string longer than MaxPromptCacheKeyLength) is recorded as
// invalid instead of failing the whole request decode: the key is an optional
// cache-routing hint, and requests carrying one were served before the gateway
// forwarded it. Parameters: data is the raw JSON value. It always returns nil.
func (k *PromptCacheKey) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	*k = PromptCacheKey{}
	if bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var value string
	if len(trimmed) == 0 || trimmed[0] != '"' || json.Unmarshal(trimmed, &value) != nil {
		k.invalid = true
		return nil
	}
	*k = NewPromptCacheKey(value)
	if k.Validate() != nil {
		*k = PromptCacheKey{invalid: true}
	}
	return nil
}

// Validate checks the client value against OpenAI's documented contract.
// It returns nil when the key is at most MaxPromptCacheKeyLength characters,
// otherwise an error wrapping ErrInvalidPromptCacheKey.
func (k PromptCacheKey) Validate() error {
	if k.invalid {
		return errors.Wrap(ErrInvalidPromptCacheKey, "prompt_cache_key must be a string")
	}
	if utf8.RuneCountInString(k.client) > MaxPromptCacheKeyLength {
		return errors.Wrapf(ErrInvalidPromptCacheKey, "prompt_cache_key must be at most %d characters", MaxPromptCacheKeyLength)
	}
	return nil
}

// Invalid reports whether the client sent a malformed key that will never be
// forwarded. It takes no parameters.
func (k PromptCacheKey) Invalid() bool {
	return k.invalid
}

// MarshalJSON encodes only the gateway-scoped forwarded value. It returns the
// forwarded key as a JSON string, or null when the key was not scoped; the
// `omitzero` tag option omits such keys from request structs entirely.
func (k PromptCacheKey) MarshalJSON() ([]byte, error) {
	if k.forwarded == "" {
		return []byte("null"), nil
	}
	encoded, err := json.Marshal(k.forwarded)
	if err != nil {
		return nil, errors.Wrap(err, "encode forwarded prompt_cache_key")
	}
	return encoded, nil
}

// IsZero reports whether the key has nothing to forward, which makes the
// `omitzero` tag option drop it from encoded requests. It returns true unless
// ScopeToUser produced a forwarded value.
func (k PromptCacheKey) IsZero() bool {
	return k.forwarded == ""
}

// Present reports whether the client supplied a non-empty, well-formed key.
// It returns false for an absent, null, empty or malformed value.
func (k PromptCacheKey) Present() bool {
	return k.client != "" && !k.invalid
}

// Forwarded returns the gateway-scoped value that will be sent upstream, or
// the empty string when the key is withheld from the destination.
func (k PromptCacheKey) Forwarded() string {
	return k.forwarded
}

// ScopeToUser derives the forwarded value for userID from the client key. It
// is idempotent: the derivation always starts from the client's value, so
// scoping a key twice (for example across MCP rounds or provider retries) never
// hashes an already-scoped value. Parameters: userID is the gateway user that
// owns the request. It returns true when a key will be forwarded.
func (k *PromptCacheKey) ScopeToUser(userID int) bool {
	if !k.Present() {
		k.forwarded = ""
		return false
	}
	k.forwarded = NamespacePromptCacheKey(userID, k.client)
	return true
}

// Withhold clears any forwarded value so the key is not sent to the current
// destination, while keeping the client's value for a later destination.
func (k *PromptCacheKey) Withhold() {
	k.forwarded = ""
}

// NamespacePromptCacheKey derives the opaque prompt_cache_key the gateway
// forwards for a client key. Parameters: userID is the gateway user and
// clientKey is the client's prompt_cache_key. It returns 32 lowercase hex
// characters of a domain-separated SHA-256 over both values, so one user and
// key always map to one cache partition, distinct users never share one, and
// the client's raw key is never disclosed upstream.
func NamespacePromptCacheKey(userID int, clientKey string) string {
	digest := sha256.New()
	// Field separators are NUL bytes and the user id is decimal, so no
	// (user, key) pair can be re-split into a different pair.
	digest.Write([]byte(promptCacheKeyNamespaceDomain))
	digest.Write([]byte{0})
	digest.Write([]byte(strconv.Itoa(userID)))
	digest.Write([]byte{0})
	digest.Write([]byte(clientKey))
	return hex.EncodeToString(digest.Sum(nil)[:promptCacheKeyNamespaceDigestBytes])
}
