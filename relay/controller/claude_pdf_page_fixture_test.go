package controller

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// Documented 2026 provider facts used as an independent oracle by the page
// policy tests. They intentionally do not reference production constants.
// https://platform.claude.com/docs/en/build-with-claude/pdf-support: each page
// is rasterized and its text extracted; text "typically" costs 1,500-3,000
// tokens per page; at most 600 pages per request.
// https://platform.claude.com/docs/en/build-with-claude/vision: the
// high-resolution image tier tops out at 4,784 tokens per image.
// https://platform.claude.com/docs/en/build-with-claude/token-counting: the
// Claude 4.7+ tokenizer yields about 30 percent more tokens.
const (
	documentedPDFPageTextTokens  = 3000
	documentedPDFTokenizerGrowth = 130
	documentedPDFPageImageTokens = 4784
	documentedPDFMaxPages        = 600
	documentedPDFContextCeiling  = 1_000_000
	documentedPDFPageTokens      = documentedPDFPageTextTokens*documentedPDFTokenizerGrowth/100 + documentedPDFPageImageTokens
	claudePDFPageFixtureModel    = "claude-sonnet-5-5"
)

// claudePDFPageFixtureOptions selects structural variants of a generated PDF.
// ObjectStream packs every dictionary into one Flate object stream with a
// cross-reference stream (PDF 1.5); otherwise a classic xref table is used.
// EscapedNames writes /Type /Page with #xx name escapes and interleaved comments.
// SharedKids omits /Type from one page object and references it Pages times.
type claudePDFPageFixtureOptions struct {
	Pages        int
	LineText     string
	Lines        int
	ObjectStream bool
	EscapedNames bool
	SharedKids   bool
	OmitCount    bool
}

// claudePDFPageFixture builds a structurally valid PDF whose pages all share one
// Flate-compressed, text-dense content stream. Every offset, /Length, /First
// and cross-reference entry is derived from the emitted bytes.
func claudePDFPageFixture(t testing.TB, options claudePDFPageFixtureOptions) []byte {
	t.Helper()
	require.Positive(t, options.Pages)
	var content strings.Builder
	content.WriteString("BT /F1 2 Tf 2 TL 8 786 Td\n")
	for line := 0; line < options.Lines; line++ {
		fmt.Fprintf(&content, "(%s) Tj T*\n", options.LineText)
	}
	content.WriteString("ET\n")
	compressedContent := claudePDFPageFixtureDeflate(t, []byte(content.String()))

	pageObjects := options.Pages
	if options.SharedKids {
		pageObjects = 1
	}
	// Object numbers: 1 catalog, 2 pages, 3 font, 4.. page objects, then content.
	contentNumber := 4 + pageObjects
	kids := make([]string, 0, options.Pages)
	for i := 0; i < options.Pages; i++ {
		number := 4 + i
		if options.SharedKids {
			number = 4
		}
		kids = append(kids, fmt.Sprintf("%d 0 R", number))
	}
	count := fmt.Sprintf(" /Count %d", options.Pages)
	if options.OmitCount {
		count = ""
	}
	dictionaries := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s]%s >>", strings.Join(kids, " "), count),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	pageType := "/Type /Page "
	if options.EscapedNames {
		pageType = "/Typ#65 %page marker\n/P#61ge "
	}
	if options.SharedKids {
		pageType = ""
	}
	for i := 0; i < pageObjects; i++ {
		dictionaries = append(dictionaries, fmt.Sprintf(
			"<< %s/Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
			pageType, contentNumber))
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	offsets := map[int]int{}
	writeObject := func(number int, body []byte) {
		offsets[number] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", number)
		pdf.Write(body)
		pdf.WriteString("\nendobj\n")
	}
	streamBody := func(dictionary string, data []byte) []byte {
		var body bytes.Buffer
		fmt.Fprintf(&body, "<< %s /Length %d >>\nstream\n", dictionary, len(data))
		body.Write(data)
		body.WriteString("\nendstream")
		return body.Bytes()
	}
	if !options.ObjectStream {
		for i, dictionary := range dictionaries {
			writeObject(i+1, []byte(dictionary))
		}
		writeObject(contentNumber, streamBody("/Filter /FlateDecode", compressedContent))
		size := contentNumber + 1
		xref := pdf.Len()
		fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", size)
		for number := 1; number < size; number++ {
			fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[number])
		}
		fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", size, xref)
		return pdf.Bytes()
	}

	writeObject(contentNumber, streamBody("/Filter /FlateDecode", compressedContent))
	var header, objects bytes.Buffer
	for i, dictionary := range dictionaries {
		fmt.Fprintf(&header, "%d %d ", i+1, objects.Len())
		objects.WriteString(dictionary)
		objects.WriteString("\n")
	}
	packed := append(header.Bytes(), objects.Bytes()...)
	objectStreamNumber := contentNumber + 1
	writeObject(objectStreamNumber, streamBody(fmt.Sprintf("/Type /ObjStm /N %d /First %d /Filter /FlateDecode",
		len(dictionaries), header.Len()), claudePDFPageFixtureDeflate(t, packed)))

	xrefNumber := objectStreamNumber + 1
	size := xrefNumber + 1
	xrefOffset := pdf.Len()
	offsets[xrefNumber] = xrefOffset
	var rows bytes.Buffer
	writeRow := func(kind byte, field2 uint32, field3 uint16) {
		rows.WriteByte(kind)
		_ = binary.Write(&rows, binary.BigEndian, field2)
		_ = binary.Write(&rows, binary.BigEndian, field3)
	}
	writeRow(0, 0, 65535)
	for i := range dictionaries {
		writeRow(2, uint32(objectStreamNumber), uint16(i))
	}
	for number := contentNumber; number < size; number++ {
		writeRow(1, uint32(offsets[number]), 0)
	}
	fmt.Fprintf(&pdf, "%d 0 obj\n", xrefNumber)
	pdf.Write(streamBody(fmt.Sprintf("/Type /XRef /Size %d /W [1 4 2] /Root 1 0 R /Filter /FlateDecode", size),
		claudePDFPageFixtureDeflate(t, rows.Bytes())))
	pdf.WriteString("\nendobj\n")
	fmt.Fprintf(&pdf, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return pdf.Bytes()
}

// claudePDFPageFixtureDeflate returns zlib-wrapped Flate bytes for a PDF stream.
func claudePDFPageFixtureDeflate(t testing.TB, data []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer, err := zlib.NewWriterLevel(&encoded, zlib.BestCompression)
	require.NoError(t, err)
	_, err = writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encoded.Bytes()
}

// claudePDFPageDocument wraps base64 PDF data in a native Claude document block.
func claudePDFPageDocument(data string) map[string]any {
	return map[string]any{
		"type": "document", "title": "Synthetic PDF", "context": "Read this fixture.",
		"source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": data},
	}
}

// claudePDFPageRequest returns a one-message native request carrying documents
// directly or inside one tool result.
func claudePDFPageRequest(documents []any, nested bool) *ClaudeMessagesRequest {
	content := documents
	if nested {
		content = []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": documents}}
	}
	return &ClaudeMessagesRequest{Model: claudePDFPageFixtureModel, MaxTokens: 1,
		Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: content}}}
}

// claudePDFPageMetadataTokens counts a manually specified provider-visible
// metadata view with the independent text tokenizer. It never calls the
// production document projection or allowance helpers under test.
func claudePDFPageMetadataTokens(t testing.TB, documents []any) int {
	t.Helper()
	parts := make([]relaymodel.MessageContent, 0, len(documents))
	for _, value := range documents {
		document := value.(map[string]any)
		source := document["source"].(map[string]any)
		metadata := map[string]any{"type": "document", "source": map[string]any{"type": source["type"]}}
		if mediaType, present := source["media_type"]; present {
			metadata["source"].(map[string]any)["media_type"] = mediaType
		}
		for _, key := range []string{"title", "context", "citations"} {
			if value, present := document[key]; present {
				metadata[key] = value
			}
		}
		encoded, err := json.Marshal(metadata)
		require.NoError(t, err)
		text := string(encoded)
		parts = append(parts, relaymodel.MessageContent{Type: "text", Text: &text})
	}
	return openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "user", Content: parts}}, claudePDFPageFixtureModel)
}

// documentedPDFPageQuote returns the independent documented page-cost oracle
// for a request carrying the supplied number of pages, capped at the
// documented per-request page limit and largest documented context window.
func documentedPDFPageQuote(pages int) int {
	pages = min(pages, documentedPDFMaxPages)
	return min(pages*documentedPDFPageTokens, documentedPDFContextCeiling)
}
