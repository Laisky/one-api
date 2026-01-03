package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectStreamBodyTerminatesOnResponseCompleted(t *testing.T) {
	t.Parallel()
	stream := strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		"event: response.completed",
		`data: {"type":"response.completed","status":"completed"}`,
		"",
	}, "\n")

	data, err := collectStreamBody(strings.NewReader(stream), 1024)
	require.NoError(t, err)
	require.Contains(t, string(data), "\"response.completed\"")
}
