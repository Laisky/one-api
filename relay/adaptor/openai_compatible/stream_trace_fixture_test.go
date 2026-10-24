package openai_compatible

import (
	"testing"

	"github.com/Laisky/one-api/common/config"
)

// configureIssue599TraceFixture isolates transport tests from synchronous SQL tracing.
func configureIssue599TraceFixture(t *testing.T) {
	t.Helper()
	previous := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = previous })
}
