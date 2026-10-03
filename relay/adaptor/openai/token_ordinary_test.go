package openai

import (
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/one-api/internal/tokenizer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// ordinaryTokenCorpus returns deterministic Unicode, special-token, invalid UTF-8 and byte-level regression cases.
func ordinaryTokenCorpus() []string {
	corpus := []string{
		"", "hello world", " ", "\r\n\t\x00", "don't I'LL café e\u0301",
		"中文 日本語 한국어 العربية हिन्दी 🙂👩🏽‍💻",
		"<|endoftext|>", "before<|endoftext|>after<|endoftext|>",
		"<|fim_prefix|>x<|fim_middle|>y<|fim_suffix|>",
		"<|endofprompt|><|im_start|>user<|im_end|>",
		"\xff\xc0\xaf\xed\xa0\x80\xf0\x9f", "a\u200db\u2028c\u2029d",
		"{\"messages\":[{\"role\":\"user\",\"content\":\"hello 世界\"}]}",
		strings.Repeat("hello 世界🙂|", 1024), strings.Repeat("x", 8192),
	}
	for value := range 256 {
		corpus = append(corpus, string([]byte{byte(value)}))
	}
	random := rand.New(rand.NewSource(427))
	for range 1024 {
		data := make([]byte, random.Intn(257))
		for i := range data {
			data[i] = byte(random.Intn(256))
		}
		corpus = append(corpus, string(data))
	}
	return corpus
}

// TestOrdinaryTokenCountingEquivalent compares complete token sequences and production counts against the previous path.
func TestOrdinaryTokenCountingEquivalent(t *testing.T) {
	previous := config.ApproximateTokenEnabled
	config.ApproximateTokenEnabled = false
	t.Cleanup(func() { config.ApproximateTokenEnabled = previous })
	corpus := ordinaryTokenCorpus()
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		t.Run(name, func(t *testing.T) {
			encoder, err := loadEncoder(name)
			require.NoError(t, err)
			for index, text := range corpus {
				legacy := encoder.Encode(text, nil, nil)
				require.Equalf(t, legacy, encoder.EncodeOrdinary(text), "corpus case %d", index)
				require.Equalf(t, len(legacy), getTokenNum(encoder, text), "billing case %d", index)
			}
		})
	}
}

// TestOrdinaryTokenCountingConcurrent verifies shared encoders preserve exact counts under simultaneous streaming calls.
func TestOrdinaryTokenCountingConcurrent(t *testing.T) {
	previous := config.ApproximateTokenEnabled
	config.ApproximateTokenEnabled = false
	t.Cleanup(func() { config.ApproximateTokenEnabled = previous })
	corpus := ordinaryTokenCorpus()[:15]
	for _, model := range []string{"gpt-4", "gpt-4o-mini", "unknown-provider-model"} {
		encoder := getTokenEncoder(model)
		require.NotNil(t, encoder)
		want := make([]int, len(corpus))
		for i, text := range corpus {
			want[i] = len(encoder.Encode(text, nil, nil))
		}
		results := make([][]int, 16)
		var workers sync.WaitGroup
		for worker := range results {
			workers.Go(func() {
				results[worker] = make([]int, len(corpus))
				for i, text := range corpus {
					results[worker][i] = CountTokenText(text, model)
				}
			})
		}
		workers.Wait()
		for _, result := range results {
			require.Equal(t, want, result, model)
		}
	}
}

// TestOrdinaryTokenCountingFallback preserves the existing byte-based approximate and unavailable-encoder behavior.
func TestOrdinaryTokenCountingFallback(t *testing.T) {
	previous := config.ApproximateTokenEnabled
	t.Cleanup(func() { config.ApproximateTokenEnabled = previous })
	for _, text := range []string{"", "hello", "世界🙂", "\xff\xc0"} {
		want := int(float64(len(text)) * 0.38)
		config.ApproximateTokenEnabled = true
		require.Equal(t, want, getTokenNum(&tiktoken.Tiktoken{}, text))
		config.ApproximateTokenEnabled = false
		require.Equal(t, want, getTokenNum(nil, text))
	}
}

var ordinaryTokenBenchmarkCount int

// BenchmarkOrdinaryTokenCounting isolates encoder work; end-to-end acceptance still requires the black-box paired study.
func BenchmarkOrdinaryTokenCounting(b *testing.B) {
	for _, name := range []string{"cl100k_base", "o200k_base"} {
		encoder, err := loadEncoder(name)
		require.NoError(b, err)
		text := "request/000001:hello 世界🙂|" + strings.Repeat("x", 96)
		for _, ordinary := range []bool{false, true} {
			variant := "legacy"
			if ordinary {
				variant = "ordinary"
			}
			b.Run(name+"/"+variant, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				for b.Loop() {
					if ordinary {
						ordinaryTokenBenchmarkCount = len(encoder.EncodeOrdinary(text))
					} else {
						ordinaryTokenBenchmarkCount = len(encoder.Encode(text, nil, nil))
					}
				}
			})
		}
	}
}
