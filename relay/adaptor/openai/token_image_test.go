package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountImageTokens_HighDetail_Square_1024_4o(t *testing.T) {
	// 1024x1024 => tiles = ceil(1024/512)*ceil(1024/512)=2*2=4
	// gpt-4.1 family: base=85, tile=170 => 4*170+85=765
	old := getImageSizeFn
	getImageSizeFn = func(_ string) (int, int, error) { return 1024, 1024, nil }
	defer func() { getImageSizeFn = old }()

	got, err := countImageTokens("https://example.com/img.jpg", "high", "gpt-4.1")
	require.NoError(t, err)
	want := 4*170 + 85
	require.Equal(t, want, got)
}

func TestCountImageTokens_HighDetail_2048x4096_4o(t *testing.T) {
	// Scale to fit in 2048 square and shortest side to 768 => tiles = 2*3=6, tokens=6*170+85=1105
	old := getImageSizeFn
	getImageSizeFn = func(_ string) (int, int, error) { return 2048, 4096, nil }
	defer func() { getImageSizeFn = old }()

	got, err := countImageTokens("u", "high", "gpt-4.1")
	require.NoError(t, err)
	want := 6*170 + 85
	require.Equal(t, want, got)
}

func TestCountImageTokens_LowDetail_Flat(t *testing.T) {
	// Low detail uses base only
	got, err := countImageTokens("u", "low", "gpt-4.1")
	require.NoError(t, err)
	require.Equal(t, 85, got)

	got, err = countImageTokens("u", "low", "gpt-4o-mini")
	require.NoError(t, err)
	require.Equal(t, 2833, got)
}

func TestCountImageTokens_ModelFamilies(t *testing.T) {
	old := getImageSizeFn
	getImageSizeFn = func(_ string) (int, int, error) { return 1024, 1024, nil }
	defer func() { getImageSizeFn = old }()

	// gpt-5: base 70, tile 140 => 4*140+70=630
	got, err := countImageTokens("u", "high", "gpt-5-chat-latest")
	require.NoError(t, err)
	require.Equal(t, 4*140+70, got, "gpt-5")

	// o1/o3: base 75, tile 150 => 4*150+75=675
	got, err = countImageTokens("u", "high", "o3")
	require.NoError(t, err)
	require.Equal(t, 4*150+75, got, "o3")

	// computer-use-preview: base 65, tile 129
	got, err = countImageTokens("u", "high", "computer-use-preview")
	require.NoError(t, err)
	require.Equal(t, 4*129+65, got, "computer-use-preview")
}
