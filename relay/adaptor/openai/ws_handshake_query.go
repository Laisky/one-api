package openai

import (
	"net/url"
	"slices"
)

// callerHandshakeQuery is a caller's WebSocket handshake query parsed only as
// input to gateway decisions. Provider handshakes never relay it: each URL
// builder copies the few values the gateway decided on into a fresh query, so
// a caller cannot smuggle provider-side selectors (`call_id`, `deployment`,
// `api-version`, case variants of `model`, duplicate `intent` values) past the
// gateway's routing, billing and credential decisions.
type callerHandshakeQuery struct {
	// values holds every well-formed caller pair.
	values url.Values
	// malformed reports that at least one pair could not be parsed; such pairs
	// are excluded from values and are never forwarded.
	malformed bool
}

// parseCallerHandshakeQuery parses the caller's raw handshake query.
//
// Parameters:
//   - rawQuery: the caller's raw query string, as received.
//
// Returns:
//   - callerHandshakeQuery: the well-formed pairs plus whether any pair was
//     malformed. url.ParseQuery keeps every well-formed pair and reports only the
//     first malformed one; that error is handled by recording it here, because a
//     malformed pair is simply never forwarded.
func parseCallerHandshakeQuery(rawQuery string) callerHandshakeQuery {
	values, err := url.ParseQuery(rawQuery)
	return callerHandshakeQuery{values: values, malformed: err != nil}
}

// unforwarded counts the caller parameters that the provider handshake does
// not carry verbatim, for DEBUG diagnostics. Only a count is reported: caller
// query keys and values may carry credentials and are never logged.
//
// Parameters:
//   - forwarded: the query the gateway actually sends to the provider.
//
// Returns:
//   - int: caller pairs that were dropped or rewritten, plus one when any
//     malformed pair was discarded.
func (q callerHandshakeQuery) unforwarded(forwarded url.Values) int {
	count := 0
	for key, values := range q.values {
		for _, value := range values {
			if !slices.Contains(forwarded[key], value) {
				count++
			}
		}
	}
	if q.malformed {
		count++
	}
	return count
}
