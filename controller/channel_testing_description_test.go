package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestModelDescriptionAllowsSummarizationCapability verifies that a general
// Chat model is not rejected merely because its description lists text
// summarization among several supported capabilities.
// Parameters: t is the test handle used to run assertions.
// Returns: no values.
func TestModelDescriptionAllowsSummarizationCapability(t *testing.T) {
	t.Parallel()

	cfg := adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "General-purpose Chat model supporting text summarization and question answering.",
	}
	require.True(t, modelConfigSupportsTextTest(cfg, true))
}
