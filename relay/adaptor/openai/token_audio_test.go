package openai

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestCountTokenMessages_AudioAccumulationBug(t *testing.T) {
	// Create a dummy ffprobe that returns a fixed duration
	tmpDir, err := os.MkdirTemp("", "test-ffprobe")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ffprobePath := filepath.Join(tmpDir, "ffprobe")
	// This script returns "10.0" (seconds)
	err = os.WriteFile(ffprobePath, []byte("#!/bin/sh\necho 10.0"), 0755)
	require.NoError(t, err)

	// Add tmpDir to PATH
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

	InitTokenEncoders()

	// 10 seconds * 10 tokens/sec = 100 tokens per audio
	audioData := base64.StdEncoding.EncodeToString([]byte("fake audio data"))

	messages := []model.Message{
		{
			Role: "user",
			Content: []model.MessageContent{
				{
					Type: "input_audio",
					InputAudio: &model.InputAudio{
						Data: audioData,
					},
				},
			},
		},
		{
			Role:    "assistant",
			Content: "Hello",
		},
		{
			Role:    "user",
			Content: "How are you?",
		},
	}

	// Expected tokens:
	// tokensPerMessage = 3
	// 3 messages: 3 * 3 = 9
	// Roles: user(1), assistant(1), user(1) = 3
	// Contents:
	//   msg 1: audio (100 tokens)
	//   msg 2: "Hello" (1 token)
	//   msg 3: "How are you?" (3 tokens)
	// Total content tokens: 100 + 1 + 3 = 104
	// Prime: 3
	// Total: 9 + 3 + 104 + 3 = 119 (actual ~120 due to rounding/tokenization)
	//
	// BUT because of the bug:
	// Msg 1: tokenNum += 3. audioTokens = 100. totalAudioTokens = 100. tokenNum += 100. role = 1. -> 104
	// Msg 2: tokenNum += 3. "Hello" = 1. totalAudioTokens = 100. tokenNum += 100. role = 1. -> 104 + 105 = 209
	// Msg 3: tokenNum += 3. "How are you?" = 3. totalAudioTokens = 100. tokenNum += 100. role = 1. -> 209 + 107 = 316
	// Total: 316 + 3 = 319

	ctx := context.Background()
	tokenCount := CountTokenMessages(ctx, messages, "gpt-4o")

	t.Logf("Token count: %d", tokenCount)

	// If the bug exists, tokenCount will be much larger than 120
	require.Equal(t, 120, tokenCount, "Token count should be 120")
}
