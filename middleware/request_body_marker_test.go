package middleware

import (
	stderrors "errors"
	"fmt"
	"io"
	"strings"
	"testing"

	stackerrors "github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestBoundedBodyLimitErrorCarriesStack verifies the limit error retains its cause, text and detection frame.
func TestBoundedBodyLimitErrorCarriesStack(t *testing.T) {
	body := &boundedBody{reader: io.NopCloser(strings.NewReader("over")), remaining: 0}
	_, err := body.Read(make([]byte, 1))
	require.ErrorIs(t, err, ErrRequestBodyTooLarge)
	var trace interface{ StackTrace() stackerrors.StackTrace }
	require.True(t, stderrors.As(err, &trace))
	require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), "boundedBody).Read")
	require.EqualError(t, err, "request body exceeds the configured size limit")
}
