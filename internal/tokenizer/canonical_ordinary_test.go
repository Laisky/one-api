package tiktoken

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	original "github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

// canonicalCorpus returns fixed, exhaustive short, and deterministic random compatibility cases.
func canonicalCorpus() []string {
	out := []string{"", "  a", " \t a", "\r\n\t\n ", "don't I'LL WE'RE they'd", "'ſ 'S 's İ ı K",
		"中文 日本語 한국어 العربية हिन्दी e\u0301 🙂👩🏽‍💻", "<|endoftext|><|fim_prefix|><|endofprompt|>",
		"\xff\xfe\xc0\xaf\xed\xa0\x80\xf0\x9f", "\u0085\u00a0\u2007\u202f\u3000a", "1234567890/abc",
		strings.Repeat("The quick brown fox 012345 中文 🙂\n", 512), strings.Repeat("x", 2048)}
	alphabet := []byte{' ', '\t', '\r', '\n', 'a', 'A', '1', '\'', 's', '/'}
	level := []string{""}
	for range 4 {
		next := make([]string, 0, len(level)*len(alphabet))
		for _, prefix := range level {
			for _, suffix := range alphabet {
				next = append(next, prefix+string(suffix))
			}
		}
		out = append(out, next...)
		level = next
	}
	for value := range 256 {
		out = append(out, string([]byte{byte(value)}), "x"+string([]byte{byte(value)})+" 's")
	}
	rng := rand.New(rand.NewSource(42720260925))
	for range 2048 {
		data := make([]byte, rng.Intn(129))
		for i := range data {
			data[i] = byte(rng.Intn(256))
		}
		out = append(out, string(data))
	}
	for range 2048 {
		var text strings.Builder
		for range rng.Intn(65) {
			text.WriteRune(rune(rng.Intn(unicode.MaxRune + 1)))
		}
		out = append(out, text.String())
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if unicode.IsSpace(r) {
			for _, suffix := range []string{"", "a", " a", "\n", "\r\na", "🙂", "'s"} {
				out = append(out, string(r)+suffix, " "+string(r)+suffix, string(r)+string(r)+suffix)
			}
		}
	}
	return out
}

// originalPieces collects independent regexp2 matches, including original invalid-byte replacement.
func originalPieces(t *testing.T, text string, pattern *regexp2.Regexp) []string {
	t.Helper()
	out := []string{}
	match, err := pattern.FindStringMatch(text)
	require.NoError(t, err)
	for match != nil {
		out = append(out, match.String())
		match, err = pattern.FindNextMatch(match)
		require.NoError(t, err)
	}
	return out
}

// TestCanonicalOrdinaryDifferential compares every piece and full token sequence with the pinned upstream oracle.
func TestCanonicalOrdinaryDifferential(t *testing.T) {
	corpus := canonicalCorpus()
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		t.Run(name, func(t *testing.T) {
			enc, err := GetEncoding(name)
			require.NoError(t, err)
			oracle, err := original.GetEncoding(name)
			require.NoError(t, err)
			require.NotNil(t, enc.bpe.canonicalOrdinary)
			pattern := regexp2.MustCompile(enc.pbeEncoding.PatStr, regexp2.None)
			for i, text := range corpus {
				normalized := text
				if !utf8.ValidString(text) {
					normalized = string([]rune(text))
				}
				pieces := []string{}
				for len(normalized) > 0 {
					n := canonicalPieceLength(normalized, enc.bpe.canonicalOrdinary)
					require.Positivef(t, n, "case %d: %q", i, text)
					pieces = append(pieces, normalized[:n])
					normalized = normalized[n:]
				}
				require.Equalf(t, originalPieces(t, text, pattern), pieces, "piece case %d: %q", i, text)
				require.Equalf(t, oracle.EncodeOrdinary(text), enc.EncodeOrdinary(text), "token case %d: %q", i, text)
			}
			t.Logf("verified %d independent piece/full-token inputs", len(corpus))
		})
	}
}

// TestCanonicalSpecialAndFallback preserves special-token behavior and noncanonical custom encodings.
func TestCanonicalSpecialAndFallback(t *testing.T) {
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		enc, err := GetEncoding(name)
		require.NoError(t, err)
		oracle, err := original.GetEncoding(name)
		require.NoError(t, err)
		for _, text := range []string{"", "a<|endoftext|>b", "<|endofprompt|>中文", "x<|endoftext|><|endoftext|>"} {
			for _, allowed := range [][]string{nil, {"all"}, {"<|endoftext|>"}} {
				require.Equal(t, oracle.Encode(text, allowed, nil), enc.Encode(text, allowed, nil))
			}
		}
		require.Panics(t, func() { enc.Encode("<|endoftext|>", nil, []string{"all"}) })
		for _, pattern := range []string{`(?s).`, `[a-z]+`, `\s+|\S+`, canonicalClPattern + `|ZZZ`} {
			bpe, err := NewCoreBPE(enc.pbeEncoding.MergeableRanks, enc.pbeEncoding.SpecialTokens, pattern)
			require.NoError(t, err)
			require.Nil(t, bpe.canonicalOrdinary)
			oldBPE, err := original.NewCoreBPE(enc.pbeEncoding.MergeableRanks, enc.pbeEncoding.SpecialTokens, pattern)
			require.NoError(t, err)
			old := original.NewTiktoken(oldBPE, &original.Encoding{}, nil)
			for _, text := range []string{" a 'S🙂\xff", "abc123\n\txyz", "", "\xff\xfe"} {
				require.Equal(t, old.EncodeOrdinary(text), bpe.encodeOrdinaryNative(text), pattern)
			}
		}
	}
}

// TestCanonicalYieldBudget requires synchronous scheduling opportunities without changing token IDs.
func TestCanonicalYieldBudget(t *testing.T) {
	enc, err := GetEncoding("o200k_base")
	require.NoError(t, err)
	for _, length := range []int{0, 1, 4095, 4096, 4097, 8192, 32768} {
		text := strings.Repeat("a ", (length+1)/2)[:length]
		calls := 0
		actual := enc.bpe.encodeCanonicalOrdinary(text, func() { calls++ })
		require.Equal(t, enc.Encode(text, nil, nil), actual)
		if length <= canonicalYieldBytes {
			require.Zero(t, calls, "short input must not yield")
		} else if length >= 2*canonicalYieldBytes {
			require.Positive(t, calls, "long input must yield before returning")
		}
		pieces := originalPieces(t, text, enc.bpe.tlRegex)
		budget, expectedCalls := 0, 0
		for i, piece := range pieces {
			budget += len(piece)
			if budget >= canonicalYieldBytes && i+1 < len(pieces) {
				expectedCalls++
				budget = 0
			}
		}
		require.Equal(t, expectedCalls, calls, "yield only between complete pieces while input remains")
	}
}

// TestCanonicalConcurrent checks shared matchers, independent request state and complete output under concurrency.
func TestCanonicalConcurrent(t *testing.T) {
	enc, err := GetEncoding("o200k_base")
	require.NoError(t, err)
	texts := []string{strings.Repeat("hello 世界🙂|", 512), " ", "'ſ'S'd\xff", "<|endoftext|>", ""}
	oracle, err := original.GetEncoding("o200k_base")
	require.NoError(t, err)
	want := make([][]int, len(texts))
	for i, text := range texts {
		want[i] = oracle.EncodeOrdinary(text)
	}
	results := make([][][]int, 16)
	var group sync.WaitGroup
	for worker := range results {
		group.Go(func() {
			for _, text := range texts {
				results[worker] = append(results[worker], enc.EncodeOrdinary(text))
			}
		})
	}
	group.Wait()
	for _, actual := range results {
		require.Equal(t, want, actual)
	}
}

// BenchmarkCanonicalOrdinary reports allocations for the same full-token API; E2E remains the adoption gate.
func BenchmarkCanonicalOrdinary(b *testing.B) {
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		enc, err := GetEncoding(name)
		require.NoError(b, err)
		oracle, err := original.GetEncoding(name)
		require.NoError(b, err)
		for _, repeats := range []int{1, 1024} {
			text := strings.Repeat("request 012345 hello 世界🙂; the quick brown fox\n", repeats)
			for label, encode := range map[string]func(string) []int{"baseline": oracle.EncodeOrdinary, "candidate": enc.EncodeOrdinary} {
				b.Run(fmt.Sprintf("%s/%d/%s", name, repeats, label), func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(text)))
					for b.Loop() {
						canonicalBenchmarkSink = encode(text)
					}
				})
			}
		}
	}
}

var canonicalBenchmarkSink []int
