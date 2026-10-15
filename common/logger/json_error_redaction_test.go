package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// TestRedactJSONErrorValueNumeric takes t and verifies real numeric decode errors retain safe wrapper context, stack and causal identity in actual JSON error fields.
func TestRedactJSONErrorValueNumeric(t *testing.T) {
	for _, scenario := range []struct {
		name, literal string
		integer       bool
	}{
		{"float_overflow", "918273645546372819e9999", false},
		{"integer_overflow", "918273645546372819999", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var raw error
			if scenario.integer {
				var target int64
				raw = json.Unmarshal([]byte(scenario.literal), &target)
			} else {
				var target any
				raw = json.Unmarshal([]byte(scenario.literal), &target)
			}
			require.Error(t, raw)
			wrapped := errors.Wrap(raw, "decode synthetic provider receipt")
			originalMessage, originalVerbose := wrapped.Error(), fmt.Sprintf("%+v", wrapped)
			safe := RedactJSONErrorValue(wrapped)
			require.ErrorIs(t, safe, raw)
			var typed *json.UnmarshalTypeError
			require.ErrorAs(t, safe, &typed)
			require.Contains(t, typed.Value, scenario.literal, "the original cause remains available to application logic")
			require.Equal(t, originalMessage, wrapped.Error())
			require.Equal(t, originalVerbose, fmt.Sprintf("%+v", wrapped))
			var output bytes.Buffer
			core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.ErrorLevel)
			zap.New(core).Error("synthetic decoder failure", zap.Error(safe))
			var fields map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &fields))
			require.NotContains(t, output.String(), scenario.literal)
			require.Contains(t, fmt.Sprint(fields["error"]), "number [REDACTED]")
			require.Contains(t, fmt.Sprint(fields["error"]), "decode synthetic provider receipt")
			require.Contains(t, fmt.Sprint(fields["errorVerbose"]), "json_error_redaction_test.go", "safe formatting must retain the original stack")
			require.Contains(t, fmt.Sprint(fields["errorVerbose"]), "decode synthetic provider receipt")
			require.NotContains(t, fmt.Sprintf("%q", safe), scenario.literal)
		})
	}
}

// TestRedactJSONErrorValueOrdinary takes t and verifies this explicit helper preserves unrelated, syntax and ordinary type diagnostics exactly.
func TestRedactJSONErrorValueOrdinary(t *testing.T) {
	var syntax any
	syntaxErr := json.Unmarshal([]byte("{"), &syntax)
	for _, original := range []error{nil, errors.New("ordinary provider failure 12345"), syntaxErr, &json.UnmarshalTypeError{Value: "string", Type: reflect.TypeFor[int64]()}, &json.UnmarshalTypeError{Value: "number", Type: reflect.TypeFor[int64]()}} {
		safe := RedactJSONErrorValue(original)
		require.Equal(t, original, safe)
		if original != nil {
			require.Equal(t, original.Error(), safe.Error())
			require.False(t, strings.Contains(safe.Error(), "[REDACTED]"))
		}
	}
}

// TestRedactJSONErrorValuePreservesShortLiteralContext takes t and verifies a short overflow value is redacted only in the typed decoder diagnostic, preserving identical ordinary wrapper and stack text.
func TestRedactJSONErrorValuePreservesShortLiteralContext(t *testing.T) {
	var target int8
	raw := json.Unmarshal([]byte("128"), &target)
	require.Error(t, raw)
	wrapped := errors.Wrap(raw, "decode release128 receipt")
	safe := RedactJSONErrorValue(wrapped)
	require.ErrorIs(t, safe, raw)
	require.Contains(t, safe.Error(), "decode release128 receipt")
	require.Contains(t, safe.Error(), "number [REDACTED]")
	require.NotContains(t, safe.Error(), "number 128")
	require.Equal(t, strings.ReplaceAll(fmt.Sprintf("%+v", wrapped), raw.Error(), "json: cannot unmarshal number [REDACTED] into Go value of type int8"), fmt.Sprintf("%+v", safe))
	var output bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.ErrorLevel)
	zap.New(core).Error("synthetic short numeric overflow", zap.Error(safe))
	var fields map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &fields))
	require.Contains(t, fmt.Sprint(fields["error"]), "release128")
	require.Contains(t, fmt.Sprint(fields["errorVerbose"]), "release128")
	require.Contains(t, fmt.Sprint(fields["errorVerbose"]), "json_error_redaction_test.go")
	require.NotContains(t, output.String(), "number 128")
}
