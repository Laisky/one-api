package aiproxy

import (
	"strconv"
	"testing"
)

func TestAIProxyDocuments2Markdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		documents []LibraryDocument
		want      string
	}{
		{
			name: "empty",
			want: "",
		},
		{
			name: "single document",
			documents: []LibraryDocument{
				{Title: "Doc One", URL: "https://example.test/doc-one"},
			},
			want: "\n\nReference Documents:\n1. [Doc One](https://example.test/doc-one)\n",
		},
		{
			name: "preserves order and raw markdown characters",
			documents: []LibraryDocument{
				{Title: "First [raw] title", URL: "https://example.test/a_(b)"},
				{Title: "Second", URL: "https://example.test/second"},
			},
			want: "\n\nReference Documents:\n1. [First [raw] title](https://example.test/a_(b))\n2. [Second](https://example.test/second)\n",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := aiProxyDocuments2Markdown(test.documents); got != test.want {
				t.Fatalf("aiProxyDocuments2Markdown() = %q, want %q", got, test.want)
			}
		})
	}
}

var benchmarkAIProxyDocumentsMarkdown string

func BenchmarkAIProxyDocuments2Markdown(b *testing.B) {
	for _, documentCount := range []int{1, 10, 100} {
		documents := make([]LibraryDocument, documentCount)
		for i := range documents {
			n := strconv.Itoa(i + 1)
			documents[i] = LibraryDocument{
				Title: "Reference document " + n + " with a representative title",
				URL:   "https://example.test/library/reference/" + n + "?source=benchmark",
			}
		}

		b.Run(strconv.Itoa(documentCount)+"_documents", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(aiProxyDocuments2Markdown(documents))))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkAIProxyDocumentsMarkdown = aiProxyDocuments2Markdown(documents)
			}
		})
	}
}
