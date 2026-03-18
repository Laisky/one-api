package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldRetryClaudeInvalidThinkingSignature(t *testing.T) {
	t.Parallel()

	t.Run("matches anthropic invalid signature error", func(t *testing.T) {
		body := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"messages.1.content.0: Invalid ` + "`signature`" + ` in ` + "`thinking`" + ` block"}}`)
		assert.True(t, shouldRetryClaudeInvalidThinkingSignature(400, body))
	})

	t.Run("ignores other invalid request errors", func(t *testing.T) {
		body := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"messages.0.content: invalid format"}}`)
		assert.False(t, shouldRetryClaudeInvalidThinkingSignature(400, body))
	})

	t.Run("ignores non 400 status", func(t *testing.T) {
		body := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"messages.1.content.0: Invalid ` + "`signature`" + ` in ` + "`thinking`" + ` block"}}`)
		assert.False(t, shouldRetryClaudeInvalidThinkingSignature(500, body))
	})
}
