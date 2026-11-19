package controller

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// behaviorDiffReplacement maps one concrete fixture value to a stable placeholder.
type behaviorDiffReplacement struct {
	value       string
	placeholder string
}

// behaviorDiffNormalizer rewrites run-varying fixture values (generated UUIDs,
// generated keys, wall-clock timestamps) to stable placeholders. Every entry is
// seeded from a value the fixture actually produced — this is normalization *by
// the fixture*, never a blind regex over "things that look like a uuid".
type behaviorDiffNormalizer struct {
	strs []behaviorDiffReplacement
	nums []behaviorDiffReplacement
}

// addString registers a string value whose every occurrence (including inside a
// larger message) is rewritten to placeholder.
// Parameters:
//   - value: the concrete value produced by the fixture.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addString(value, placeholder string) {
	if value == "" {
		return
	}
	n.strs = append(n.strs, behaviorDiffReplacement{value: value, placeholder: placeholder})
}

// addNumber registers a numeric value that is rewritten to placeholder on an
// exact match of its JSON text.
// Parameters:
//   - value: the concrete numeric value produced by the fixture.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addNumber(value int64, placeholder string) {
	if value == 0 {
		return
	}
	n.nums = append(n.nums, behaviorDiffReplacement{value: fmt.Sprintf("%d", value), placeholder: placeholder})
}

// addChangedNumber registers a numeric value only when the handler actually
// changed it away from the fixture's pinned value. Registering unconditionally
// would let a placeholder mask a field the handler left alone, which would
// silently weaken the capture.
// Parameters:
//   - original: the value the fixture pinned before the request.
//   - current: the value observed after the request.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addChangedNumber(original, current int64, placeholder string) {
	if current == original {
		return
	}
	n.addNumber(current, placeholder)
}

// addTimestampWindow registers every second-resolution timestamp the handler
// could have observed, offset by a known constant. Used only where a response
// field is computed as helper.GetTimestamp()+offset and never persisted (the
// ConsumeToken pre-hold expiry is zeroed in the database on confirmation), so
// the exact value cannot be read back from a row. The candidate set is bounded
// by the wall clock measured around this very request.
// Parameters:
//   - f: fixture carrying the measured request window.
//   - offset: constant added to the handler's timestamp.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addTimestampWindow(f *behaviorDiffFixture, offset int64, placeholder string) {
	for ts := f.requestStart; ts <= f.requestEnd; ts++ {
		n.addNumber(ts+offset, placeholder)
	}
}

// apply rewrites a decoded JSON tree in place, replacing known fixture values.
// Parameters:
//   - v: decoded JSON value (map, slice, string, json.Number, bool or nil).
//
// Return values:
//   - any: the normalized value.
func (n *behaviorDiffNormalizer) apply(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		for key, val := range typed {
			typed[key] = n.apply(val)
		}
		return typed
	case []any:
		for i, val := range typed {
			typed[i] = n.apply(val)
		}
		return typed
	case string:
		out := canonicalizeGoStructFieldPath(typed)
		for _, r := range n.sortedStrings() {
			out = strings.ReplaceAll(out, r.value, r.placeholder)
		}
		return out
	case json.Number:
		text := typed.String()
		for _, r := range n.nums {
			if text == r.value {
				return r.placeholder
			}
		}
		return typed
	default:
		return v
	}
}

// goStructFieldPathPattern matches the struct path encoding/json embeds in an
// unmarshal error: `Go struct field <path> of type <type>`.
var goStructFieldPathPattern = regexp.MustCompile(`Go struct field ([A-Za-z0-9_.]+) of type `)

// canonicalizeGoStructFieldPath reduces the struct path in an encoding/json
// unmarshal error to its final segment, i.e. the JSON field name.
//
// The full path is a Go-internal detail whose spelling depends on the toolchain:
// for a field promoted from an embedded struct, Go 1.26 renders
// `channelPayload.Channel.type` while Go 1.27 renders `channelPayload.type`.
// Pinning either spelling makes this suite fail on the other toolchain without
// any behavior having changed. The parts a client can actually act on — the
// failing field name and the expected Go type — are preserved, so a real change
// to which field rejects the payload still breaks the baseline.
//
// Parameters:
//   - value: a candidate response string.
//
// Return values:
//   - string: value with any struct path reduced to its final segment.
func canonicalizeGoStructFieldPath(value string) string {
	return goStructFieldPathPattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := goStructFieldPathPattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		path := groups[1]
		if idx := strings.LastIndex(path, "."); idx >= 0 {
			path = path[idx+1:]
		}
		return "Go struct field " + path + " of type "
	})
}

// sortedStrings returns the string replacements ordered longest-value-first so a
// shorter value can never consume a prefix of a longer one.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffReplacement: replacements in descending value length.
func (n *behaviorDiffNormalizer) sortedStrings() []behaviorDiffReplacement {
	out := make([]behaviorDiffReplacement, len(n.strs))
	copy(out, n.strs)
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].value) > len(out[j].value) })
	return out
}

// behaviorDiffNormalizeToken registers the run-varying columns of a token row
// (generated uuid/key plus wall-clock timestamps written by the handler).
// Parameters:
//   - t: active test handle.
//   - name: token name to look up.
//   - n: normalizer to extend.
//
// Return values: none.
func behaviorDiffNormalizeToken(t *testing.T, name string, n *behaviorDiffNormalizer) {
	t.Helper()
	var row model.Token
	require.NoError(t, model.DB.Where("name = ?", name).First(&row).Error)
	n.addString(row.UUID, "<new-token-uuid>")
	n.addString(row.Key, "<new-token-key>")
	n.addNumber(row.CreatedTime, "<ts>")
	n.addNumber(row.AccessedTime, "<ts>")
	n.addNumber(row.CreatedAt, "<ts-milli>")
	n.addNumber(row.UpdatedAt, "<ts-milli>")
}

// behaviorDiffNormalizeUserIfCreated registers the generated identifiers of a
// user row only if the handler actually created one. It exists for the
// wrong-type probes: if a tree rejects the body no row appears (nothing to
// normalize), and if a tree accepts it the row's generated uuid is normalized —
// so the pre/post difference shows up in the response body, never as a
// normalization artifact.
// Parameters:
//   - t: active test handle.
//   - username: username the probe attempted to create.
//   - n: normalizer to extend.
//
// Return values: none.
func behaviorDiffNormalizeUserIfCreated(t *testing.T, username string, n *behaviorDiffNormalizer) {
	t.Helper()
	var rows []model.User
	require.NoError(t, model.DB.Where("username = ?", username).Find(&rows).Error)
	for _, row := range rows {
		n.addString(row.UUID, "<new-user-uuid>")
		n.addNumber(row.CreatedAt, "<ts-milli>")
		n.addNumber(row.UpdatedAt, "<ts-milli>")
	}
}
