package tiktoken

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	upstream "github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

var coreStorageSink *CoreBPE

// TestCoreStoragePreservesPublicBehavior compares complete ordinary/special token IDs and decoded bytes against upstream.
func TestCoreStoragePreservesPublicBehavior(t *testing.T) {
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		t.Run(name, func(t *testing.T) {
			actual, err := GetEncoding(name)
			require.NoError(t, err)
			reference, err := upstream.GetEncoding(name)
			require.NoError(t, err)
			inputs := []string{"", "ASCII words and 123456", "中文 日本語 한국어 العربية हिन्दी 🙂👩🏽‍💻", "\xff\xc0\xaf\xed\xa0\x80", " \t\r\n\u0085\u00a0\u2028", "'S 'ſ 'RE I'LL", strings.Repeat("long-text 中文🙂 ", 1024)}
			rng := rand.New(rand.NewSource(4270926))
			for range 512 {
				raw := make([]byte, rng.Intn(513))
				_, err = rng.Read(raw)
				require.NoError(t, err)
				inputs = append(inputs, string(raw))
			}
			for index, input := range inputs {
				expected := reference.EncodeOrdinary(input)
				got := actual.EncodeOrdinary(input)
				require.Equal(t, expected, got, "input %d", index)
				require.Equal(t, reference.Decode(expected), actual.Decode(got), "decode %d", index)
			}
			for _, input := range []string{"a <|endoftext|> b", "<|endofprompt|>", "<|endoftext|><|endoftext|>"} {
				expected := reference.Encode(input, []string{"all"}, nil)
				require.Equal(t, expected, actual.Encode(input, []string{"all"}, nil))
				require.Equal(t, reference.Decode(expected), actual.Decode(expected))
				require.Panics(t, func() { actual.Encode(input, nil, []string{"all"}) })
			}
			var wg sync.WaitGroup
			observed := make([][]int, 64)
			for i := range observed {
				wg.Go(func() { observed[i] = actual.EncodeOrdinary(inputs[i+1]) })
			}
			wg.Wait()
			for i, got := range observed {
				require.Equal(t, reference.EncodeOrdinary(inputs[i+1]), got)
			}
		})
	}
}

// TestCoreStoragePreservesConstruction checks nil/custom ranks and constructor errors independently of canonical patterns.
func TestCoreStoragePreservesConstruction(t *testing.T) {
	for _, tc := range []struct {
		name, pattern string
		ranks         map[string]int
	}{
		{"empty", `.`, nil},
		{"duplicate ranks", `.`, map[string]int{"a": 1, "b": 1}},
		{"invalid regex", `[`, map[string]int{"a": 1}},
		{"custom", `[ab ]+`, map[string]int{"a": 0, "b": 1, " ": 2, "ab": 3, "ba": 4, "aba": 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			special := map[string]int{"<|s|>": 256}
			currentCore, currentErr := NewCoreBPE(tc.ranks, special, tc.pattern)
			oldCore, oldErr := upstream.NewCoreBPE(tc.ranks, special, tc.pattern)
			if oldErr != nil {
				require.Error(t, currentErr)
				require.Equal(t, oldErr.Error(), currentErr.Error())
				require.Nil(t, currentCore)
				return
			}
			require.NoError(t, currentErr)
			current := NewTiktoken(currentCore, &Encoding{}, map[string]any{"<|s|>": true})
			original := upstream.NewTiktoken(oldCore, &upstream.Encoding{}, map[string]any{"<|s|>": true})
			for _, input := range []string{"", "a", "abababa", " ab ba ", "a<|s|>b"} {
				want := original.EncodeOrdinary(input)
				require.Equal(t, want, current.EncodeOrdinary(input))
				want = original.Encode(input, []string{"all"}, nil)
				require.Equal(t, want, current.Encode(input, []string{"all"}, nil))
				require.Equal(t, original.Decode(want), current.Decode(want))
			}
			require.Equal(t, original.Decode([]int{999, 256, 0}), current.Decode([]int{999, 256, 0}))
		})
	}
}

// TestCoreStorageProbe is an opt-in isolated-process measurement, not a mandatory machine-specific CI threshold.
func TestCoreStorageProbe(t *testing.T) {
	name := os.Getenv("ONEAPI_CORE_STORAGE_PROBE")
	if name == "" {
		t.Skip("isolated diagnostic requires explicit encoding name")
	}
	require.Contains(t, []string{"cl100k_base", "o200k_base"}, name)
	encoding, err := getEncoding(name)
	require.NoError(t, err)
	runtime.GC()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	coreStorageSink, err = NewCoreBPE(encoding.MergeableRanks, encoding.SpecialTokens, encoding.PatStr)
	elapsed := time.Since(started)
	require.NoError(t, err)
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	data := map[string]any{"encoding": name, "entries": len(encoding.MergeableRanks), "retained_heap_bytes": int64(after.HeapAlloc) - int64(before.HeapAlloc), "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "constructor_ns": elapsed.Nanoseconds()}
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	fmt.Printf("CORE_STORAGE_PROBE %s\n", encoded)
	runtime.KeepAlive(coreStorageSink)
	runtime.KeepAlive(encoding)
}

// BenchmarkCoreStorageConstruction measures startup allocations after dictionary loading, not streaming throughput.
func BenchmarkCoreStorageConstruction(b *testing.B) {
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		b.Run(name, func(b *testing.B) {
			encoding, err := getEncoding(name)
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				coreStorageSink, err = NewCoreBPE(encoding.MergeableRanks, encoding.SpecialTokens, encoding.PatStr)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
