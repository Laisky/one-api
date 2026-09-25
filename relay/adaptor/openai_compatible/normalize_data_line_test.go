package openai_compatible

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var normalizedDataSink string

// legacyNormalizeDataLine expresses the original normalization contract independently of the fast path.
func legacyNormalizeDataLine(data string) string {
	if strings.HasPrefix(data, "data:") {
		return "data: " + strings.TrimLeft(data[len("data:"):], " ")
	}
	return data
}

// TestNormalizeDataLineEquivalence covers prefix spacing, arbitrary bytes, empty values and Unicode content.
func TestNormalizeDataLineEquivalence(t *testing.T) {
	inputs := []string{"", "data:", "data: ", "data:x", "data:   x", "data:\tx", "data: \tx", "data: \u00a0x", "data: \r", ": ping", "event: x", "Data: x"}
	rng := rand.New(rand.NewPCG(427, 2))
	for i := 0; i < 1024; i++ {
		data := make([]byte, rng.IntN(256))
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		inputs = append(inputs, string(data), "data:"+strings.Repeat(" ", i%8)+string(data))
	}
	for _, input := range inputs {
		require.Equal(t, legacyNormalizeDataLine(input), NormalizeDataLine(input))
	}
}

// BenchmarkNormalizeDataLineCopies compares canonical-input copies in the reference and current implementation.
func BenchmarkNormalizeDataLineCopies(b *testing.B) {
	for name, normalize := range map[string]func(string) string{"legacy": legacyNormalizeDataLine, "current": NormalizeDataLine} {
		b.Run(name, func(b *testing.B) {
			payload := "data: " + strings.Repeat("content", 64)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				normalizedDataSink = normalize(payload)
			}
		})
	}
}
