package openai

import (
	"testing"

	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

// TestCl100kModelsShareEncoderInstance reproduces the duplicate-encoder memory
// waste. gpt-3.5-turbo and gpt-4 both use the cl100k_base encoding, therefore
// they MUST be backed by a single shared *tiktoken.Tiktoken instance. On the
// pre-fix code InitTokenEncoders built two independent cl100k CoreBPE objects
// (one for gpt-3.5, one for gpt-4), wasting ~7 MiB of resident heap. This test
// FAILS on the pre-fix code (distinct pointers) and PASSES after de-duplication.
func TestCl100kModelsShareEncoderInstance(t *testing.T) {
	InitTokenEncoders()

	enc35 := getTokenEncoder("gpt-3.5-turbo")
	enc4 := getTokenEncoder("gpt-4")
	require.NotNil(t, enc35)
	require.NotNil(t, enc4)
	require.Same(t, enc35, enc4,
		"gpt-3.5-turbo and gpt-4 both use cl100k_base and must share ONE encoder instance (no duplicate CoreBPE)")
}

// TestEncoderTokenCountsStable guards that keying the cache by encoding name does
// not change token counts (billing correctness). The shared cl100k encoder must
// produce identical counts for both model aliases and must match a freshly built
// cl100k_base encoder.
func TestEncoderTokenCountsStable(t *testing.T) {
	InitTokenEncoders()

	const text = "The quick brown fox jumps over the lazy dog. 你好，世界！ 1234567890"

	enc35 := getTokenEncoder("gpt-3.5-turbo")
	enc4 := getTokenEncoder("gpt-4")
	require.Equal(t,
		len(enc35.Encode(text, nil, nil)),
		len(enc4.Encode(text, nil, nil)),
		"both cl100k models must count identically")

	freshCl100k, err := tiktoken.GetEncoding("cl100k_base")
	require.NoError(t, err)
	require.Equal(t,
		len(freshCl100k.Encode(text, nil, nil)),
		len(enc4.Encode(text, nil, nil)),
		"shared cl100k encoder must match a freshly built cl100k_base encoder")
}

// TestO200kModelResolvesToO200k guards that gpt-4o-class models still resolve to
// the o200k_base encoding (a distinct instance from cl100k), so the refactor does
// not collapse everything onto one encoding.
func TestO200kModelResolvesToO200k(t *testing.T) {
	InitTokenEncoders()

	const text = "hello world from a gpt-4o request"

	enc4o := getTokenEncoder("gpt-4o")
	require.NotNil(t, enc4o)

	freshO200k, err := tiktoken.GetEncoding("o200k_base")
	require.NoError(t, err)
	require.Equal(t,
		len(freshO200k.Encode(text, nil, nil)),
		len(enc4o.Encode(text, nil, nil)),
		"gpt-4o must resolve to o200k_base")

	require.NotSame(t, getTokenEncoder("gpt-3.5-turbo"), enc4o,
		"cl100k and o200k must be distinct encoder instances")
}
