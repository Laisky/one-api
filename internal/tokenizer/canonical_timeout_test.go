package tiktoken

import (
	"math"
	"testing"
	"time"

	"github.com/dlclark/regexp2"
	original "github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

// TestCanonicalConfiguredTimeoutKeepsOracle requires finite timeout policies to retain the original matcher.
// The global is changed only in this nonparallel test and restored before other tests run.
func TestCanonicalConfiguredTimeoutKeepsOracle(t *testing.T) {
	previous := regexp2.DefaultMatchTimeout
	t.Cleanup(func() { regexp2.DefaultMatchTimeout = previous })
	ranks := make(map[string]int, 256)
	for value := range 256 {
		ranks[string([]byte{byte(value)})] = value
	}
	for _, pattern := range []string{canonicalClPattern, canonicalO2Pattern} {
		for _, timeout := range []time.Duration{time.Duration(math.MaxInt64), time.Minute, 0, -time.Second} {
			regexp2.DefaultMatchTimeout = timeout
			core, err := NewCoreBPE(ranks, map[string]int{}, pattern)
			require.NoError(t, err)
			require.Equal(t, timeout, core.tlRegex.MatchTimeout)
			if timeout == time.Duration(math.MaxInt64) {
				require.NotNil(t, core.canonicalOrdinary)
				continue
			}
			require.Nil(t, core.canonicalOrdinary, "a finite timeout cannot use a timeout-free matcher")
			// A generous finite deadline makes the full-output comparison deterministic;
			// zero/expired deadlines above verify dispatch without timing-sensitive assertions.
			if timeout != time.Minute {
				continue
			}
			oldCore, err := original.NewCoreBPE(ranks, map[string]int{}, pattern)
			require.NoError(t, err)
			oracle := original.NewTiktoken(oldCore, &original.Encoding{}, nil)
			for _, text := range []string{"", "  a 's'ſ 世界🙂", "\xff\xfe\r\n", "0123456789"} {
				require.Equal(t, oracle.EncodeOrdinary(text), core.encodeOrdinaryNative(text))
			}
		}
	}
}
