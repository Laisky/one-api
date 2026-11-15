package model

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// validateExactBoundsTokens checks whole numeric tokens at the expected schema paths.
func validateExactBoundsTokens(wire []byte, schemaPath ...string) error {
	for _, check := range []struct {
		path []string
		want string
	}{
		{[]string{"properties", "count", "minimum"}, "9007199254740993"},
		{[]string{"properties", "count", "maximum"}, "9007199254740995"},
		{[]string{"properties", "nested", "items", "minimum"}, "9007199254740993.125"},
	} {
		path := append(append([]string(nil), schemaPath...), check.path...)
		token, err := exactSchemaTokenAtPath(wire, path)
		if err != nil {
			return errors.Wrap(err, "locate exact schema bound")
		}
		if !bytes.Equal(bytes.TrimSpace(token), []byte(check.want)) {
			return errors.Errorf("%s: expected numeric token %s, got %s", strings.Join(path, "."), check.want, string(token))
		}
	}
	return nil
}

// exactSchemaTokenAtPath returns an unrounded JSON token or a diagnostic for its missing/malformed path.
func exactSchemaTokenAtPath(wire []byte, path []string) (json.RawMessage, error) {
	node := json.RawMessage(wire)
	for i, key := range path {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(node, &object); err != nil {
			return nil, errors.Wrapf(err, "decode JSON path %s", strings.Join(path[:i], "."))
		}
		child, ok := object[key]
		if !ok {
			return nil, errors.Errorf("missing JSON path %s", strings.Join(path[:i+1], "."))
		}
		node = child
	}
	return node, nil
}

// TestExactBoundsAssertionMutationControls rejects rounded individual bounds while accepting healthy numeric tokens.
func TestExactBoundsAssertionMutationControls(t *testing.T) {
	for _, tc := range []struct{ name, from, to, path string }{
		{"healthy", "", "", ""},
		{"rounded-minimum", `"minimum":9007199254740993,`, `"minimum":9007199254740992,`, "properties.count.minimum"},
		{"rounded-maximum", `"maximum":9007199254740995`, `"maximum":9007199254740996`, "properties.count.maximum"},
		{"rounded-fraction", `"minimum":9007199254740993.125`, `"minimum":9007199254740994`, "properties.nested.items.minimum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := exactBoundsSchema
			if tc.from != "" {
				schema = strings.Replace(schema, tc.from, tc.to, 1)
			}
			wire := []byte(`{"schema":` + schema + `}`)
			if tc.name == "rounded-minimum" {
				require.True(t, strings.Contains(string(wire), `"minimum":9007199254740993`) && strings.Contains(string(wire), `"maximum":9007199254740995`) && strings.Contains(string(wire), `"minimum":9007199254740993.125`), "the old substring check is fooled by the untouched decimal prefix")
			}
			err := validateExactBoundsTokens(wire, "schema")
			if tc.name == "healthy" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.path)
			}
		})
	}
}
