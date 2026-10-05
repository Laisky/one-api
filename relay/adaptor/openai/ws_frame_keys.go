package openai

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
)

// responseWSGuardedKeys are the response.create root keys whose values the
// WebSocket guards authorize, pin, or strip before forwarding a frame.
var responseWSGuardedKeys = []string{"type", "model", "previous_response_id", "conversation"}

// realtimeWSGuardedRootKeys are the realtime client-event root keys the
// session-update model guard dispatches on or inspects.
var realtimeWSGuardedRootKeys = []string{"type", "session", "input_audio_transcription"}

// validateWSFrameGuardedKeys rejects a JSON object whose root names any guarded
// key more than once (after JSON escape decoding and case folding) or under a
// non-canonical spelling. The WebSocket guards decode frames into maps that keep
// only the last exact key, while a provider may read the first duplicate or fold
// key case, so an ambiguous frame would let the gateway check one value and the
// provider act on another. Keys are scanned with common.ScanJSONRootKeys, never
// byte substrings, so escaped spellings cannot hide a duplicate.
// Parameters: object is a raw JSON payload; guarded lists canonical key names.
// Returns: nil when every guarded key appears at most once and canonically, or
// when object is not one valid JSON object (callers decode it themselves and
// never forward a parsed view of it); otherwise an error wrapping
// common.ErrAmbiguousJSONKey.
func validateWSFrameGuardedKeys(object []byte, guarded ...string) error {
	keys, err := common.ScanJSONRootKeys(object)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool, len(guarded))
	for _, key := range keys {
		for _, canonical := range guarded {
			if !common.JSONKeyMatches(key, canonical) {
				continue
			}
			if key != canonical {
				return errors.Wrapf(common.ErrAmbiguousJSONKey, "parameter %q must be spelled %q", key, canonical)
			}
			if seen[canonical] {
				return errors.Wrapf(common.ErrAmbiguousJSONKey, "parameter %q is sent more than once", canonical)
			}
			seen[canonical] = true
		}
	}
	return nil
}
