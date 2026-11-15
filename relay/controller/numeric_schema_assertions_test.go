package controller

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// numericSchemaTokenExpectation identifies one exact numeric bound in a schema.
type numericSchemaTokenExpectation struct {
	path    []string
	token   string
	rounded string
}

// geminiNumericTokenChecks returns the exact inclusive bound tokens expected by the Gemini fixture.
func geminiNumericTokenChecks() []numericSchemaTokenExpectation {
	return []numericSchemaTokenExpectation{
		{[]string{"properties", "count", "minimum"}, "9007199254740993", "9007199254740992"},
		{[]string{"properties", "count", "maximum"}, "9007199254740997", "9007199254740996"},
		{[]string{"properties", "nested", "properties", "ratio", "minimum"}, "0.1", "0.10000000000000002"},
		{[]string{"properties", "nested", "properties", "ratio", "maximum"}, "0.9", "0.9000000000000001"},
	}
}

// openAINumericTokenChecks returns the exact inclusive and exclusive tokens expected by the OpenAI fixture.
func openAINumericTokenChecks() []numericSchemaTokenExpectation {
	return []numericSchemaTokenExpectation{
		{[]string{"properties", "count", "minimum"}, "9007199254740993", "9007199254740992"},
		{[]string{"properties", "count", "maximum"}, "9007199254740997", "9007199254740996"},
		{[]string{"properties", "nested", "items", "properties", "ratio", "exclusiveMinimum"}, "0.123456789012345678901234567890", "0.12345678901234568"},
		{[]string{"properties", "nested", "items", "properties", "ratio", "exclusiveMaximum"}, "0.987654321098765432109876543210", "0.9876543210987654"},
	}
}

// storedMCPNumericTokenChecks returns the exact stored schema tokens checked in ordinary and deferred injection.
func storedMCPNumericTokenChecks() []numericSchemaTokenExpectation {
	return []numericSchemaTokenExpectation{
		{[]string{"properties", "count", "minimum"}, "9007199254740993", "9007199254740992"},
		{[]string{"properties", "count", "maximum"}, "9007199254740997", "9007199254740996"},
		{[]string{"properties", "nested", "properties", "threshold", "minimum"}, "9007199254740999", "9007199254741000"},
		{[]string{"properties", "nested", "properties", "threshold", "maximum"}, "9007199254741003", "9007199254741004"},
	}
}

// validateNumericSchemaTokens compares whole RawMessage tokens at the specified
// schema paths and returns diagnostics without invoking test assertions.
func validateNumericSchemaTokens(wire []byte, checks []numericSchemaTokenExpectation) error {
	for _, check := range checks {
		token, err := numericSchemaTokenAtPath(wire, check.path)
		if err != nil {
			return errors.Wrap(err, "locate numeric schema bound")
		}
		if !bytes.Equal(bytes.TrimSpace(token), []byte(check.token)) {
			return errors.Errorf("%s: expected numeric token %s, got %s", strings.Join(check.path, "."), check.token, string(token))
		}
	}
	return nil
}

// numericSchemaTokenAtPath reads one exact JSON token without floating-point decoding.
func numericSchemaTokenAtPath(wire []byte, path []string) (json.RawMessage, error) {
	node := json.RawMessage(wire)
	for i, key := range path {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(node, &object); err != nil {
			return nil, errors.Wrapf(err, "decode schema path %s", strings.Join(path[:i], "."))
		}
		child, ok := object[key]
		if !ok {
			return nil, errors.Errorf("missing schema path %s", strings.Join(path[:i+1], "."))
		}
		node = child
	}
	return node, nil
}

// TestNumericBoundsTokenMutationControls rejects every rounded target despite an exact decimal decoy elsewhere.
func TestNumericBoundsTokenMutationControls(t *testing.T) {
	for _, fixture := range []struct {
		name, schema string
		checks       []numericSchemaTokenExpectation
	}{
		{"gemini", geminiNumericBoundsSchema, geminiNumericTokenChecks()},
		{"openai", openAIStandardBoundsSchema, openAINumericTokenChecks()},
		{"stored-mcp", storedMCPNumericSchema, storedMCPNumericTokenChecks()},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			schema := strings.TrimSuffix(fixture.schema, "}") + `,"default":{"minimum":9007199254740993.125}}`
			require.NoError(t, validateNumericSchemaTokens([]byte(schema), fixture.checks))
			for i, check := range fixture.checks {
				t.Run(strings.Join(check.path, "."), func(t *testing.T) {
					field := check.path[len(check.path)-1]
					mutated := strings.Replace(schema, `"`+field+`":`+check.token, `"`+field+`":`+check.rounded, 1)
					if i == 0 {
						for _, oldCheck := range fixture.checks {
							require.Contains(t, mutated, `"`+oldCheck.path[len(oldCheck.path)-1]+`":`+oldCheck.token, "the legacy whole-schema token checks can be fooled by another path")
						}
					}
					require.ErrorContains(t, validateNumericSchemaTokens([]byte(mutated), fixture.checks), strings.Join(check.path, "."))
				})
			}
		})
	}
}
