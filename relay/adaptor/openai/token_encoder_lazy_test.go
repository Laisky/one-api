package openai

import (
	"sync"
	"testing"

	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

// encodingLoadedForTest reports whether an encoding has already been built and
// cached. It reads the package cache under the read lock.
func encodingLoadedForTest(name string) bool {
	encoderMu.RLock()
	defer encoderMu.RUnlock()
	_, ok := encoderByName[name]
	return ok
}

func resetEncoderCacheForTest() {
	encoderMu.Lock()
	defer encoderMu.Unlock()
	encoderByName = make(map[string]*tiktoken.Tiktoken)
}

// TestEncodersLoadLazily is the acceptance test for lazy loading: after startup
// only the default encoder (cl100k_base) is resident; o200k_base is NOT built
// until a gpt-4o-class model is actually counted. This proves the eager-load
// waste (building o200k at boot even when never used) is gone.
func TestEncodersLoadLazily(t *testing.T) {
	resetEncoderCacheForTest()
	t.Cleanup(resetEncoderCacheForTest)

	InitTokenEncoders()
	require.True(t, encodingLoadedForTest("cl100k_base"),
		"InitTokenEncoders must warm the default cl100k_base encoder for fast-fail")
	require.False(t, encodingLoadedForTest("o200k_base"),
		"o200k_base must NOT be loaded until a gpt-4o-class model is used (lazy loading)")

	// Counting a cl100k model must not trigger o200k either.
	_ = getTokenEncoder("gpt-3.5-turbo")
	require.False(t, encodingLoadedForTest("o200k_base"))

	// First gpt-4o-class request loads o200k_base on demand.
	enc4o := getTokenEncoder("gpt-4o")
	require.NotNil(t, enc4o)
	require.True(t, encodingLoadedForTest("o200k_base"),
		"o200k_base must be loaded on first gpt-4o-class request")
}

// TestEncoderBuiltOncePerEncoding verifies that repeated lookups across many
// models sharing an encoding reuse a single cached instance (no per-call or
// per-model rebuild).
func TestEncoderBuiltOncePerEncoding(t *testing.T) {
	resetEncoderCacheForTest()
	t.Cleanup(resetEncoderCacheForTest)
	InitTokenEncoders()

	first := getTokenEncoder("gpt-3.5-turbo")
	for _, m := range []string{"gpt-4", "gpt-4-turbo", "gpt-4-0613", "text-embedding-3-small", "some-unknown-model"} {
		require.Same(t, first, getTokenEncoder(m),
			"model %q resolves to cl100k_base and must reuse the single shared instance", m)
	}

	encoderMu.RLock()
	total := len(encoderByName)
	encoderMu.RUnlock()
	require.Equal(t, 1, total, "only cl100k_base should be resident; got %d encodings", total)
}

// TestLoadEncoderConcurrent stresses the lazy cache from many goroutines at once
// (run with -race). It proves the cache is data-race-free (the pre-fix code wrote
// the encoder map without a lock) and that each encoding is built exactly once
// and shared across all concurrent callers.
func TestLoadEncoderConcurrent(t *testing.T) {
	resetEncoderCacheForTest()
	t.Cleanup(resetEncoderCacheForTest)

	models := []string{"gpt-3.5-turbo", "gpt-4", "gpt-4o", "gpt-4.1", "text-embedding-3-small"}
	const n = 128
	results := make([]*tiktoken.Tiktoken, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = getTokenEncoder(models[i%len(models)])
		}(i)
	}
	wg.Wait()

	encoderMu.RLock()
	cl := encoderByName["cl100k_base"]
	o2 := encoderByName["o200k_base"]
	total := len(encoderByName)
	encoderMu.RUnlock()

	require.NotNil(t, cl)
	require.NotNil(t, o2)
	require.Equal(t, 2, total, "exactly cl100k_base and o200k_base must be built, got %d", total)

	for i, r := range results {
		switch models[i%len(models)] {
		case "gpt-4o", "gpt-4.1":
			require.Same(t, o2, r, "gpt-4o-class must share the single o200k_base instance")
		default:
			require.Same(t, cl, r, "cl100k models must share the single cl100k_base instance")
		}
	}
}
