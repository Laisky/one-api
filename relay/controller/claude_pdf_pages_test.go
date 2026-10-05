package controller

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// claudePDFScanFixture scans a generated fixture with an optional byte rewrite
// and returns the page estimate plus the raw signals.
func claudePDFScanFixture(t *testing.T, options claudePDFPageFixtureOptions, rewrite func([]byte) []byte) (int, claudePDFPageSignals) {
	t.Helper()
	if options.LineText == "" {
		options.LineText, options.Lines = "Synthetic page text for the page scan.", 4
	}
	pdf := claudePDFPageFixture(t, options)
	if rewrite != nil {
		pdf = rewrite(pdf)
	}
	signals := scanClaudePDFPages(pdf, newClaudePDFScanBudget())
	pages := signals.pages()
	t.Logf("PDF_PAGE_SCAN bytes=%d pages=%d page_names=%d tree_count=%d kids=%d objstm=%d/%d opaque=%q",
		len(pdf), pages, signals.pageNames, signals.pageTreeCount, signals.kidsEntries,
		signals.objStmInspected, signals.objStmNames, signals.opaqueReason)
	return pages, signals
}

// claudePDFReplace returns a rewrite that replaces exactly one occurrence of old.
func claudePDFReplace(t *testing.T, old, replacement string) func([]byte) []byte {
	return func(pdf []byte) []byte {
		require.Equal(t, 1, bytes.Count(pdf, []byte(old)), "fixture rewrite target %q", old)
		return bytes.Replace(pdf, []byte(old), []byte(replacement), 1)
	}
}

// TestClaudePDFPageScanVisibleStructures counts pages that a PDF reader would
// render for classic, object-stream, escaped-name, shared-reference and
// count-only page trees, and ignores outline counts.
func TestClaudePDFPageScanVisibleStructures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options claudePDFPageFixtureOptions
		rewrite func(*testing.T) func([]byte) []byte
		want    int
	}{
		{name: "classic_1", options: claudePDFPageFixtureOptions{Pages: 1}, want: 1},
		{name: "classic_37", options: claudePDFPageFixtureOptions{Pages: 37}, want: 37},
		{name: "object_stream_37", options: claudePDFPageFixtureOptions{Pages: 37, ObjectStream: true}, want: 37},
		{name: "unfiltered_object_stream_37", options: claudePDFPageFixtureOptions{Pages: 37, ObjectStream: true, PlainObjectStream: true, OmitCount: true}, want: 37},
		{name: "escaped_names_37", options: claudePDFPageFixtureOptions{Pages: 37, EscapedNames: true}, want: 37},
		{name: "shared_kids_no_type_no_count", options: claudePDFPageFixtureOptions{Pages: 37, SharedKids: true, OmitCount: true}, want: 37},
		{name: "shared_kids_object_stream", options: claudePDFPageFixtureOptions{Pages: 37, SharedKids: true, ObjectStream: true, OmitCount: true}, want: 37},
		{name: "escaped_kids_name", options: claudePDFPageFixtureOptions{Pages: 37, SharedKids: true, OmitCount: true},
			rewrite: func(t *testing.T) func([]byte) []byte { return claudePDFReplace(t, "/Kids [", "/K#69ds [") }, want: 37},
		{name: "count_only_behind_comment", options: claudePDFPageFixtureOptions{Pages: 1},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Count 1 ", "/C#6funt %hidden\n 250 ")
			}, want: 250},
		{name: "outline_count_ignored", options: claudePDFPageFixtureOptions{Pages: 3, OutlineCount: 500}, want: 3},
		{name: "outline_count_ignored_object_stream", options: claudePDFPageFixtureOptions{Pages: 3, OutlineCount: 500, ObjectStream: true}, want: 3},
		{name: "string_delimiters_in_object_stream_dict", options: claudePDFPageFixtureOptions{Pages: 37, ObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Type /ObjStm", "/Type /ObjStm /Note (a >> b << c \\) stream)")
			}, want: 37},
		{name: "huge_count_capped", options: claudePDFPageFixtureOptions{Pages: 1},
			rewrite: func(t *testing.T) func([]byte) []byte { return claudePDFReplace(t, "/Count 1 ", "/Count 99999999999 ") }, want: claudeNativePDFMaxPagesPerRequest},
		{name: "page_limit_capped", options: claudePDFPageFixtureOptions{Pages: 650, ObjectStream: true}, want: claudeNativePDFMaxPagesPerRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rewrite func([]byte) []byte
			if tc.rewrite != nil {
				rewrite = tc.rewrite(t)
			}
			pages, signals := claudePDFScanFixture(t, tc.options, rewrite)
			require.Empty(t, signals.opaqueReason)
			require.Equal(t, tc.want, pages)
		})
	}
}

// TestClaudePDFPageScanOpaqueStructures treats structures that can hide page
// objects from a lexical scan as using the documented per-request page limit.
func TestClaudePDFPageScanOpaqueStructures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options claudePDFPageFixtureOptions
		rewrite func(*testing.T) func([]byte) []byte
		reason  string
	}{
		{name: "encrypted", options: claudePDFPageFixtureOptions{Pages: 2},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Root 1 0 R", "/Root 1 0 R /Encrypt 99 0 R")
			}, reason: "encrypted"},
		{name: "object_stream_predictor", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Type /ObjStm", "/Type /ObjStm /DecodeParms << /Predictor 12 /Columns 5 >>")
			}, reason: "object_stream_predictor"},
		{name: "object_stream_other_filter", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Type /ObjStm", "/Type /ObjStm /Filter [/ASCIIHexDecode]")
			}, reason: "object_stream_filter"},
		{name: "object_stream_indirect_filter", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true, PlainObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Type /ObjStm", "/Type /ObjStm /Filter 99 0 R")
			}, reason: "object_stream_filter"},
		{name: "object_stream_detached_from_stream", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "endobj\nstartxref", "endobj\n%/ObjStm\nstartxref")
			},
			reason: "uninspected_object_stream"},
		{name: "unterminated_string_hides_object_stream", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "%PDF-1.5\n", "%PDF-1.5\n0 0 obj "+strings.Repeat("(", 4096)+"\n")
			},
			reason: "uninspected_object_stream"},
		{name: "indirect_kids", options: claudePDFPageFixtureOptions{Pages: 2},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return claudePDFReplace(t, "/Kids [", "/Kids 99 0 R /Unused [")
			}, reason: "indirect_kids"},
		{name: "inflate_budget", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true, ObjectStreamPadding: claudeNativePDFScanInflateBudget + 1},
			reason: "inflate_budget"},
		{name: "declared_flate_without_zlib", options: claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true},
			rewrite: func(t *testing.T) func([]byte) []byte {
				return func(pdf []byte) []byte {
					marker := []byte("/Type /ObjStm")
					start := bytes.Index(pdf, marker)
					require.Positive(t, start)
					data := start + bytes.Index(pdf[start:], []byte("stream\n")) + len("stream\n")
					corrupted := append([]byte(nil), pdf...)
					corrupted[data] = 'X'
					return corrupted
				}
			}, reason: "object_stream_zlib_header"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rewrite func([]byte) []byte
			if tc.rewrite != nil {
				rewrite = tc.rewrite(t)
			}
			pages, signals := claudePDFScanFixture(t, tc.options, rewrite)
			require.Equal(t, tc.reason, signals.opaqueReason)
			require.Equal(t, claudeNativePDFMaxPagesPerRequest, pages)
		})
	}
	random := make([]byte, 64<<10)
	_, err := rand.Read(random)
	require.NoError(t, err)
	signals := scanClaudePDFPages(random, newClaudePDFScanBudget())
	require.Equal(t, claudeNativePDFMaxPagesPerRequest, signals.pages(), "bytes without page evidence are not bounded")
	require.Equal(t, "no_page_evidence", signals.opaqueReason)
}

// TestClaudePDFPageScanSharedBudget charges every object stream in one request
// to one decompression budget so many small documents cannot multiply work.
func TestClaudePDFPageScanSharedBudget(t *testing.T) {
	pdf := claudePDFPageFixture(t, claudePDFPageFixtureOptions{Pages: 2, ObjectStream: true, LineText: "x", Lines: 1,
		ObjectStreamPadding: claudeNativePDFScanInflateBudget / 3})
	budget := newClaudePDFScanBudget()
	var reasons []string
	for range 4 {
		signals := scanClaudePDFPages(pdf, budget)
		signals.pages()
		reasons = append(reasons, signals.opaqueReason)
	}
	t.Logf("PDF_PAGE_SHARED_BUDGET reasons=%q remaining=%d", reasons, budget.inflatedBytes)
	require.Equal(t, []string{"", "", "inflate_budget", "inflate_budget"}, reasons)
}

// TestClaudePDFLexerContexts pins comment, string, escape and stream-data
// classification used to keep structural state honest: stream data reports
// only the names that matter (/H is skipped), fully lexes the tokens after
// /Kids, and ends at "endstream" even inside a longer run.
func TestClaudePDFLexerContexts(t *testing.T) {
	input := "/A % /B (x\n(/C (/D) \\) /E) /F#20G stream\n/H ) ( % /Kids [R] xendstream /I"
	lexer := newClaudePDFMemoryLexer([]byte(input))
	var got []string
	var token claudePDFToken
	for lexer.next(&token) {
		text := ""
		switch token.kind {
		case claudePDFTokenName, claudePDFTokenInteger, claudePDFTokenKeyword:
			text = string(token.head[:min(token.length, claudePDFTokenHeadSize)])
		}
		got = append(got, fmt.Sprintf("%d:%d:%s", token.context, token.kind, text))
		if token.kind == claudePDFTokenKeyword && token.is("stream") {
			lexer.enterData()
		}
	}
	require.Equal(t, strings.Join([]string{
		"0:1:A", "1:1:B", "1:3:x",
		"2:1:C", "2:1:D", "2:1:E",
		"0:1:F G", "0:3:stream",
		"3:1:Kids", "3:6:", "3:3:R", "3:7:", "3:3:x",
		"0:1:I",
	}, " "), strings.Join(got, " "))
}

// BenchmarkClaudePDFPageScan measures the raw lexical scan of a large mostly
// binary document, which bounds the admission cost per decoded byte.
func BenchmarkClaudePDFPageScan(b *testing.B) {
	pdf := claudePDFPageFixture(b, claudePDFPageFixtureOptions{Pages: 600, ObjectStream: true, LineText: "x", Lines: 1})
	random := make([]byte, 30<<20)
	_, err := rand.Read(random)
	require.NoError(b, err)
	document := append(append(append([]byte(nil), pdf...), []byte("99 0 obj\n<< /Length 1 >>\nstream\n")...), random...)
	b.SetBytes(int64(len(document)))
	b.ResetTimer()
	for range b.N {
		signals := scanClaudePDFPages(document, newClaudePDFScanBudget())
		require.Equal(b, claudeNativePDFMaxPagesPerRequest, signals.pages())
	}
}
