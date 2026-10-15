package logger

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
)

// RedactJSONErrorValue takes a caller-selected JSON decoder error and returns a log-safe view of a raw numeric value, retaining ordinary diagnostics and causal identity.
// It does not mutate the original error or change the generic alert transport; callers explicitly select this application payload policy.
func RedactJSONErrorValue(err error) error {
	var mismatch *json.UnmarshalTypeError
	if !errors.As(err, &mismatch) || mismatch == nil || !strings.HasPrefix(mismatch.Value, "number ") {
		return err
	}
	literal := strings.TrimPrefix(mismatch.Value, "number ")
	if literal == "" {
		return err
	}
	safeMismatch := *mismatch
	safeMismatch.Value = "number " + Redact(literal)
	originalDiagnostic, safeDiagnostic := mismatch.Error(), safeMismatch.Error()
	return &redactedJSONErrorValue{
		cause:   err,
		message: strings.ReplaceAll(err.Error(), originalDiagnostic, safeDiagnostic),
		verbose: strings.ReplaceAll(fmt.Sprintf("%+v", err), originalDiagnostic, safeDiagnostic),
	}
}

// redactedJSONErrorValue stores safe log formatting while preserving the untouched cause for application error inspection.
type redactedJSONErrorValue struct {
	cause            error
	message, verbose string
}

// Error takes no arguments and returns the safe original error message with the raw numeric value replaced.
func (e *redactedJSONErrorValue) Error() string { return e.message }

// Unwrap takes no arguments and returns the untouched cause for errors.Is and errors.As.
func (e *redactedJSONErrorValue) Unwrap() error { return e.cause }

// Cause takes no arguments and returns the untouched cause for pkg/errors-compatible callers.
func (e *redactedJSONErrorValue) Cause() error { return e.cause }

// Format takes a formatting state and verb, writes safe ordinary or verbose context including existing stack information, and returns no value.
func (e *redactedJSONErrorValue) Format(state fmt.State, verb rune) {
	if verb == 'v' && state.Flag('+') {
		_, _ = fmt.Fprint(state, e.verbose)
		return
	}
	if verb == 'q' {
		_, _ = fmt.Fprintf(state, "%q", e.message)
		return
	}
	_, _ = fmt.Fprint(state, e.message)
}
