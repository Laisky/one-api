package model

import (
	"strconv"
	"strings"
	"testing"
)

var benchmarkMessageStringContentResult string

// BenchmarkMessageStringContent measures aggregation cost as structured response fragmentation grows.
func BenchmarkMessageStringContent(b *testing.B) {
	for _, chunks := range []int{1, 16, 64, 256} {
		chunk := strings.Repeat("x", 128)
		content := make([]any, chunks)
		for i := range content {
			content[i] = map[string]any{
				"type": ContentTypeText,
				"text": chunk,
			}
		}
		message := Message{Content: content}
		wantLen := chunks * len(chunk)

		b.Run("chunks_"+strconv.Itoa(chunks), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchmarkMessageStringContentResult = message.StringContent()
			}
			if len(benchmarkMessageStringContentResult) != wantLen {
				b.Fatalf("result length = %d, want %d", len(benchmarkMessageStringContentResult), wantLen)
			}
		})
	}
}
