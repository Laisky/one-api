package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetTextFromSRT_PreservesLegacySelection verifies SRT parsing keeps the first text line after each timestamp.
func TestGetTextFromSRT_PreservesLegacySelection(t *testing.T) {
	t.Parallel()

	body := []byte("1\r\n00:00:00,000 --> 00:00:01,000\r\nHello\r\nIgnored second line\r\n\r\n2\r\n00:00:01,000 --> 00:00:02,000\r\nWorld\r\n")

	text, err := getTextFromSRT(body)
	require.NoError(t, err)
	require.Equal(t, "HelloWorld", text)
}

// TestGetTextFromSRT_LastLineWithoutNewline verifies the final subtitle line is preserved without a trailing newline.
func TestGetTextFromSRT_LastLineWithoutNewline(t *testing.T) {
	t.Parallel()

	body := []byte("1\n00:00:00,000 --> 00:00:01,000\nTail text")

	text, err := getTextFromSRT(body)
	require.NoError(t, err)
	require.Equal(t, "Tail text", text)
}

// TestGetTextFromVTT_UsesSubtitleRules verifies VTT extraction remains aligned with SRT extraction behavior.
func TestGetTextFromVTT_UsesSubtitleRules(t *testing.T) {
	t.Parallel()

	body := []byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nAlpha\n\n00:00:01.000 --> 00:00:02.000\nBeta\n")

	text, err := getTextFromVTT(body)
	require.NoError(t, err)
	require.Equal(t, "AlphaBeta", text)
}
