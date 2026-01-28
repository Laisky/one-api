package openai

import (
	"context"
	"encoding/base64"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
)

var (
	dummyFFProbeOnce     sync.Once
	dummyFFProbeDir      string
	dummyFFProbeDuration string
)

// withDummyFFprobe sets up a temporary ffprobe shim that returns a fixed duration.
// Parameters: t is the test handler; duration is the ffprobe output in seconds.
// Returns: a cleanup function to restore PATH and remove temp files.
func withDummyFFprobe(t *testing.T, duration string) func() {
	t.Helper()
	dummyFFProbeOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "test-ffprobe")
		require.NoError(t, err)

		ffprobePath := filepath.Join(tmpDir, "ffprobe")
		err = os.WriteFile(ffprobePath, []byte("#!/bin/sh\necho "+duration), 0755)
		require.NoError(t, err)

		dummyFFProbeDir = tmpDir
		dummyFFProbeDuration = duration
	})
	if dummyFFProbeDuration != "" && dummyFFProbeDuration != duration {
		t.Fatalf("dummy ffprobe duration mismatch: have %s, want %s", dummyFFProbeDuration, duration)
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dummyFFProbeDir+":"+oldPath)

	return func() {
		require.NoError(t, os.Setenv("PATH", oldPath))
	}
}

// getAudioTokensPerSecond resolves audio pricing tokens per second for the model.
// Parameters: modelName is the target model name.
// Returns: the configured tokens per second or the default when not configured.
func getAudioTokensPerSecond(modelName string) float64 {
	audioCfg, found := pricing.ResolveAudioPricing(modelName, nil, &Adaptor{})
	if found && audioCfg != nil && audioCfg.PromptTokensPerSecond > 0 {
		return audioCfg.PromptTokensPerSecond
	}
	return pricing.DefaultAudioPromptTokensPerSecond
}

// TestCountTokenMessages_AudioAccumulationBug verifies audio tokens are counted once per request.
// Parameters: t is the test handler.
// Returns: nothing.
func TestCountTokenMessages_AudioAccumulationBug(t *testing.T) {
	cleanup := withDummyFFprobe(t, "10.0")
	defer cleanup()

	InitTokenEncoders()

	// 10 seconds * tokens/sec = tokens per audio
	tokensPerSecond := getAudioTokensPerSecond("gpt-4o")
	perAudioTokens := tokensPerSecond * 10
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
	//   msg 1: audio (perAudioTokens)
	//   msg 2: "Hello" (2 tokens)
	//   msg 3: "How are you?" (3 tokens)
	// Total content tokens: perAudioTokens + 2 + 3
	// Prime: 3
	// Total: 9 + 3 + (perAudioTokens + 2 + 3) + 3
	//
	// BUT because of the bug:
	// Msg 1: tokenNum += 3. audioTokens = perAudioTokens. totalAudioTokens = perAudioTokens. tokenNum += perAudioTokens. role = 1.
	// Msg 2: tokenNum += 3. "Hello" = 2. totalAudioTokens = perAudioTokens. tokenNum += perAudioTokens. role = 1.
	// Msg 3: tokenNum += 3. "How are you?" = 3. totalAudioTokens = perAudioTokens. tokenNum += perAudioTokens. role = 1.

	ctx := context.Background()
	tokenCount := CountTokenMessages(ctx, messages, "gpt-4o")

	t.Logf("Token count: %d", tokenCount)

	// If the bug exists, tokenCount will be much larger than expected
	expected := int(math.Ceil(perAudioTokens)) + 20
	require.Equal(t, expected, tokenCount, "Token count should be %d", expected)
}

// TestCountTokenMessages_MultiAudio verifies multiple audio messages are summed once per request.
// Parameters: t is the test handler.
// Returns: nothing.
func TestCountTokenMessages_MultiAudio(t *testing.T) {
	cleanup := withDummyFFprobe(t, "10.0")
	defer cleanup()

	InitTokenEncoders()

	// 10 seconds * tokens/sec = tokens per audio
	tokensPerSecond := getAudioTokensPerSecond("gpt-4o")
	perAudioTokens := tokensPerSecond * 10
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
	}

	ctx := context.Background()
	tokenCount := CountTokenMessages(ctx, messages, "gpt-4o")

	// tokensPerMessage = 3, role tokens per message = 1, two messages => 8
	// total audio tokens = perAudioTokens * 2
	// prime = 3
	expected := int(math.Ceil(perAudioTokens*2)) + 11
	require.Equal(t, expected, tokenCount, "Token count should be %d", expected)
}
