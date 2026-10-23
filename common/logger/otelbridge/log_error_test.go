package otelbridge

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

type cachedLogError struct {
	messageCalls, typeCalls *int
	kind                    string
}

func (e cachedLogError) Error() string     { *e.messageCalls++; panic("cached Error must not execute") }
func (e cachedLogError) ErrorType() string { *e.typeCalls++; return e.kind }

// TestLogFidelityCachedErrorMessage keeps the already encoded Zap value and
// derives only the missing SDK type without invoking Error again.
func TestLogFidelityCachedErrorMessage(t *testing.T) {
	var messageCalls, typeCalls int
	message := "original cached message"
	err := cachedLogError{&messageCalls, &typeCalls, "original.CustomType"}
	attrs, failure := DeriveLogExceptionAttributes(err, nil, &message)
	require.NoError(t, failure)
	require.Equal(t, []attribute.KeyValue{attribute.String("exception.message", message), attribute.String("exception.type", "original.CustomType")}, attrs)
	require.Equal(t, 0, messageCalls)
	require.Equal(t, 1, typeCalls)
	explicit := []attribute.KeyValue{attribute.String("exception.message", ""), attribute.String("exception.type", "")}
	attrs, failure = DeriveLogExceptionAttributes(err, explicit, nil)
	require.NoError(t, failure)
	require.Empty(t, attrs)
	require.Equal(t, 0, messageCalls)
	require.Equal(t, 1, typeCalls)
}

// TestLogFidelityCachedErrorFailure rejects an invalid cached message or type
// without returning a successfully derived subset.
func TestLogFidelityCachedErrorFailure(t *testing.T) {
	for _, tc := range []struct{ name, message, kind string }{
		{"message", string([]byte{255}), "valid"},
		{"type", "valid", string([]byte{255})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var messageCalls, typeCalls int
			attrs, err := DeriveLogExceptionAttributes(cachedLogError{&messageCalls, &typeCalls, tc.kind}, nil, &tc.message)
			require.ErrorIs(t, err, ErrLogEncoding)
			require.Nil(t, attrs)
			require.Equal(t, 0, messageCalls)
		})
	}
}
