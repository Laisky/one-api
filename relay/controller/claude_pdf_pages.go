package controller

import (
	"bytes"
	"compress/zlib"
	"io"
)

// Native PDF admission limits from Anthropic's published documentation
// (verified 2026-10):
//
//   - https://platform.claude.com/docs/en/build-with-claude/pdf-support:
//     "Maximum pages per request: 600 (100 when the request's context window is
//     under 1M tokens)" and "Maximum request size: 32 MB (varies by platform)";
//     Bedrock allows 20 MB and Google Cloud 30 MB
//     (https://platform.claude.com/docs/en/api/overview).
//   - https://platform.claude.com/docs/en/build-with-claude/context-windows: the
//     largest context window is 1M tokens; a request whose input exceeds the
//     model's window is rejected rather than billed.
const (
	// claudeNativePDFMaxPagesPerRequest caps the summed pages of all native PDFs in one request.
	claudeNativePDFMaxPagesPerRequest = 600
	// claudeNativePDFContextTokenCeiling caps the summed native PDF estimate at the largest documented context window.
	claudeNativePDFContextTokenCeiling = 1_000_000
	// claudeNativePDFScanMaxDecodedBytes is the largest decoded document the page scan holds in memory.
	claudeNativePDFScanMaxDecodedBytes = 32 << 20
	// claudeNativePDFScanInflateBudget bounds decompressed object-stream bytes per request.
	claudeNativePDFScanInflateBudget = 16 << 20
	// claudeNativePDFScanMaxObjectStreams bounds object-stream decompressions per request.
	claudeNativePDFScanMaxObjectStreams = 4096
	// claudeNativePDFScanChunkSize is the reusable lexer buffer for decompressed bytes.
	claudeNativePDFScanChunkSize = 32 << 10
	// claudeNativePDFScanMaxFrames bounds tracked dictionary nesting; deeper
	// dictionaries stay balanced but are not recorded.
	claudeNativePDFScanMaxFrames = 64
)

// claudePDFScanBudget bounds decompression work across every PDF in one request
// and reuses one zlib reader and one lexer chunk.
type claudePDFScanBudget struct {
	inflatedBytes int64
	objectStreams int
	inflater      io.ReadCloser
	chunk         []byte
}

// newClaudePDFScanBudget returns a fresh per-request decompression budget.
func newClaudePDFScanBudget() *claudePDFScanBudget {
	return &claudePDFScanBudget{inflatedBytes: claudeNativePDFScanInflateBudget, objectStreams: claudeNativePDFScanMaxObjectStreams}
}

// Read implements io.Reader over the active inflater while charging the shared
// budget. It reports io.ErrShortBuffer once the budget is spent so the caller
// can treat the document as opaque.
func (budget *claudePDFScanBudget) Read(buffer []byte) (int, error) {
	if budget.inflatedBytes <= 0 {
		return 0, io.ErrShortBuffer
	}
	if int64(len(buffer)) > budget.inflatedBytes {
		buffer = buffer[:budget.inflatedBytes]
	}
	n, err := budget.inflater.Read(buffer)
	budget.inflatedBytes -= int64(n)
	return n, err
}

// claudePDFPageSignals accumulates conservative page-count evidence.
//
// pageNames counts structural /Type /Page dictionaries; pageTreeCount is the
// largest /Count of a page-tree dictionary (or any /Count outside structural
// syntax); kidsEntries counts references and inline dictionaries inside /Kids
// arrays of page-tree-like dictionaries (and of any /Kids outside structural
// syntax), so a page object referenced repeatedly counts once per reference.
// objStmNames counts /ObjStm names in the raw file; objStmInspected counts those
// whose object stream was decompressed and scanned. opaqueReason names the
// first structure that hid page objects from the scan.
type claudePDFPageSignals struct {
	pageNames       int
	pageTreeCount   int
	kidsEntries     int
	objStmNames     int
	objStmInspected int
	opaqueReason    string
}

// markOpaque records the first reason the scan cannot bound the page count.
func (signals *claudePDFPageSignals) markOpaque(reason string) {
	if signals.opaqueReason == "" {
		signals.opaqueReason = reason
	}
}

// pages returns the conservative page estimate in 1..claudeNativePDFMaxPagesPerRequest.
// Documents whose structure hides page objects from the scan, or that show no
// page evidence at all, are treated as using the per-request page limit.
// It records why a document was treated as opaque in opaqueReason.
func (signals *claudePDFPageSignals) pages() int {
	if signals.objStmNames > signals.objStmInspected {
		signals.markOpaque("uninspected_object_stream")
	}
	pages := max(signals.pageNames, signals.pageTreeCount, signals.kidsEntries)
	if pages <= 0 {
		signals.markOpaque("no_page_evidence")
	}
	if signals.opaqueReason != "" {
		return claudeNativePDFMaxPagesPerRequest
	}
	return min(pages, claudeNativePDFMaxPagesPerRequest)
}

// claudePDFFrame records the facts the scanner needs about one dictionary.
// A /Count belongs to the page tree only in a dictionary with /Kids or
// /Type /Pages, which excludes outline counts. treeMarker flags name-tree,
// number-tree and form-field keys, whose /Kids are not pages unless the
// dictionary also carries a page-tree /Type or /Count.
type claudePDFFrame struct {
	count       int
	hasCount    bool
	hasKids     bool
	kidsEntries int
	typePages   bool
	treeMarker  bool
	first       bool
	objStm      int
	filter      bool
	flate       bool
	otherFilter bool
	decodeParms bool
}

// claudePDFPass holds structural state for one lexical pass: the raw file or
// one decompressed object stream.
type claudePDFPass struct {
	raw            bool
	frames         []claudePDFFrame
	untracked      int
	lastClosed     claudePDFFrame
	closedAdjacent bool
	previousKey    string
	looseCount     bool
	expectKids     bool
	kidsDepth      int
	kidsDicts      int
	kidsFrame      int
}

// scanClaudePDFPages lexes decoded PDF bytes and every object stream they
// declare, returning conservative page signals. It never follows references,
// renders pages or allocates proportionally to the document beyond the shared
// fixed-size decompression chunk.
func scanClaudePDFPages(data []byte, budget *claudePDFScanBudget) claudePDFPageSignals {
	var signals claudePDFPageSignals
	signals.scan(newClaudePDFMemoryLexer(data), true, budget)
	return signals
}

// scan runs one lexical pass to the end of input and then settles any
// dictionaries left open by truncated or malformed syntax.
func (signals *claudePDFPageSignals) scan(lexer *claudePDFLexer, raw bool, budget *claudePDFScanBudget) {
	pass := claudePDFPass{raw: raw, kidsFrame: -1}
	var token claudePDFToken
	for lexer.next(&token) {
		signals.handle(&pass, lexer, &token, budget)
	}
	signals.closeFrames(&pass)
}

// closeFrame settles one closed dictionary: page-tree counts and page-tree
// /Kids entries become signals, and filter facts propagate to the parent.
func (signals *claudePDFPageSignals) closeFrame(pass *claudePDFPass, closed claudePDFFrame) {
	if closed.typePages || closed.hasKids {
		signals.pageTreeCount = max(signals.pageTreeCount, closed.count)
	}
	if closed.kidsEntries > 0 && (closed.typePages || closed.hasCount || !closed.treeMarker) {
		signals.kidsEntries += closed.kidsEntries
	}
	if len(pass.frames) > 0 {
		parent := &pass.frames[len(pass.frames)-1]
		parent.flate = parent.flate || closed.flate
		parent.otherFilter = parent.otherFilter || closed.otherFilter
		parent.decodeParms = parent.decodeParms || closed.decodeParms
	}
	if pass.kidsFrame >= len(pass.frames) {
		pass.kidsFrame = -1
	}
}

// closeFrames settles every open dictionary, innermost first.
func (signals *claudePDFPageSignals) closeFrames(pass *claudePDFPass) {
	for len(pass.frames) > 0 {
		closed := pass.frames[len(pass.frames)-1]
		pass.frames = pass.frames[:len(pass.frames)-1]
		signals.closeFrame(pass, closed)
	}
	pass.untracked = 0
}

// handle applies one token to the conservative counters and, for structural
// tokens, to the dictionary state of pass.
func (signals *claudePDFPageSignals) handle(pass *claudePDFPass, lexer *claudePDFLexer, token *claudePDFToken, budget *claudePDFScanBudget) {
	if token.kind == claudePDFTokenName {
		switch {
		case token.is("Encrypt"):
			signals.markOpaque("encrypted")
		case pass.raw && token.is("ObjStm"):
			signals.objStmNames++
		}
	}
	signals.handleKids(pass, token)
	if token.context != claudePDFContextStructural {
		if pass.looseCount && token.kind == claudePDFTokenInteger {
			signals.pageTreeCount = max(signals.pageTreeCount, token.value)
		}
		pass.looseCount = token.kind == claudePDFTokenName && token.is("Count")
		return
	}
	pass.looseCount = false
	closedAdjacent := pass.closedAdjacent
	pass.closedAdjacent = false
	previousKey := pass.previousKey
	pass.previousKey = ""
	var frame *claudePDFFrame
	if len(pass.frames) > 0 {
		frame = &pass.frames[len(pass.frames)-1]
	}
	switch token.kind {
	case claudePDFTokenDictOpen:
		if len(pass.frames) >= claudeNativePDFScanMaxFrames {
			pass.untracked++
			return
		}
		pass.frames = append(pass.frames, claudePDFFrame{})
	case claudePDFTokenDictClose:
		if pass.untracked > 0 {
			pass.untracked--
			return
		}
		if frame == nil {
			return
		}
		closed := *frame
		pass.frames = pass.frames[:len(pass.frames)-1]
		signals.closeFrame(pass, closed)
		pass.lastClosed, pass.closedAdjacent = closed, true
	case claudePDFTokenName:
		if previousKey == "Type" && token.is("Page") {
			signals.pageNames++
		}
		if frame != nil {
			frame.recordName(token, previousKey)
			if token.is("Kids") {
				pass.kidsFrame = len(pass.frames) - 1
			}
		}
		switch {
		case token.is("Type"):
			pass.previousKey = "Type"
		case token.is("Count"):
			pass.previousKey = "Count"
		}
	case claudePDFTokenInteger:
		if frame != nil && previousKey == "Count" {
			frame.count, frame.hasCount = max(frame.count, token.value), true
		}
	case claudePDFTokenKeyword:
		switch {
		case !pass.raw:
		case token.is("obj"):
			signals.closeFrames(pass)
		case token.is("stream"):
			scanned := closedAdjacent && signals.inspectStream(pass.lastClosed, lexer, budget)
			lexer.enterData()
			if scanned {
				lexer.pos = lexer.end
			}
		}
	}
}

// handleKids counts /Kids array entries. Structural entries accrue to the
// dictionary that owns the array and are settled when it closes; entries in
// comments, strings or stream data count directly. A structural /Kids value
// that is not a direct array hides the page tree and marks the document opaque.
func (signals *claudePDFPageSignals) handleKids(pass *claudePDFPass, token *claudePDFToken) {
	if pass.expectKids {
		pass.expectKids = false
		if token.kind == claudePDFTokenArrayOpen {
			pass.kidsDepth, pass.kidsDicts = 1, 0
			if token.context != claudePDFContextStructural {
				pass.kidsFrame = -1
			}
			return
		}
		if token.context == claudePDFContextStructural {
			signals.markOpaque("indirect_kids")
		}
	}
	if pass.kidsDepth > 0 {
		entry := false
		switch token.kind {
		case claudePDFTokenArrayOpen:
			pass.kidsDepth++
		case claudePDFTokenArrayClose:
			pass.kidsDepth--
		case claudePDFTokenDictOpen:
			entry = pass.kidsDepth == 1 && pass.kidsDicts == 0
			pass.kidsDicts++
		case claudePDFTokenDictClose:
			pass.kidsDicts = max(0, pass.kidsDicts-1)
		case claudePDFTokenKeyword:
			entry = pass.kidsDepth == 1 && pass.kidsDicts == 0 && token.is("R")
		}
		if entry {
			if pass.kidsFrame >= 0 && pass.kidsFrame < len(pass.frames) {
				pass.frames[pass.kidsFrame].kidsEntries++
			} else {
				signals.kidsEntries++
			}
		}
	}
	if token.kind == claudePDFTokenName && token.is("Kids") {
		pass.expectKids, pass.kidsFrame = true, -1
	}
}

// recordName stores the stream-filter, page-tree and name-tree facts of a
// structural name inside a dictionary.
func (frame *claudePDFFrame) recordName(token *claudePDFToken, previousKey string) {
	switch {
	case previousKey == "Type" && token.is("Pages"):
		frame.typePages = true
	case token.is("Kids"):
		frame.hasKids = true
	case token.is("First"):
		frame.first = true
	case token.is("ObjStm"):
		frame.objStm++
	case token.is("Limits"), token.is("Names"), token.is("Nums"), token.is("FT"), token.is("Fields"), token.is("T"):
		frame.treeMarker = true
	case token.is("Filter"), token.is("F"):
		frame.filter = true
	case token.is("FlateDecode"), token.is("Fl"):
		frame.flate = true
	case token.is("DecodeParms"), token.is("DP"), token.is("Predictor"):
		frame.decodeParms = true
	case token.is("ASCIIHexDecode"), token.is("AHx"), token.is("ASCII85Decode"), token.is("A85"),
		token.is("LZWDecode"), token.is("LZW"), token.is("RunLengthDecode"), token.is("RL"), token.is("Crypt"),
		token.is("DCTDecode"), token.is("DCT"), token.is("JPXDecode"), token.is("JBIG2Decode"),
		token.is("CCITTFaxDecode"), token.is("CCF"):
		frame.otherFilter = true
	}
}

// inspectStream decompresses and scans an object stream whose dictionary
// directly precedes the stream keyword. Object streams the scan cannot decode
// exactly as a reader would (other filters, predictors, budget exhaustion or a
// declared Flate stream without a zlib header) mark the document opaque. It
// returns true when it already scanned the raw stream bytes structurally, so
// the caller skips them instead of counting them again as stream data.
func (signals *claudePDFPageSignals) inspectStream(frame claudePDFFrame, lexer *claudePDFLexer, budget *claudePDFScanBudget) bool {
	if frame.objStm == 0 && !frame.first {
		return false
	}
	switch {
	case frame.otherFilter, frame.filter && !frame.flate:
		// Other filters, and filters given by indirect reference, are not decoded.
		signals.markOpaque("object_stream_filter")
		return false
	case frame.decodeParms:
		signals.markOpaque("object_stream_predictor")
		return false
	case budget.objectStreams <= 0:
		signals.markOpaque("object_stream_limit")
		return false
	}
	budget.objectStreams--
	data := lexer.data[min(lexer.position(), len(lexer.data)):]
	if end := bytes.IndexAny(data[:min(len(data), 64)], "\r\n"); end >= 0 {
		if data[end] == '\r' && end+1 < len(data) && data[end+1] == '\n' {
			end++
		}
		data = data[end+1:]
	}
	if len(data) < 2 || data[0]&0x0f != 8 || data[0]>>4 > 7 || data[1]&0x20 != 0 || (uint16(data[0])<<8|uint16(data[1]))%31 != 0 {
		if frame.flate {
			signals.markOpaque("object_stream_zlib_header")
			return false
		}
		// An unfiltered object stream is plain syntax up to "endstream".
		if end := bytes.Index(data, claudePDFEndstream); end >= 0 {
			data = data[:end]
		}
		signals.scan(newClaudePDFMemoryLexer(data), false, budget)
		signals.objStmInspected += frame.objStm
		return true
	}
	var err error
	if budget.inflater == nil {
		budget.inflater, err = zlib.NewReader(bytes.NewReader(data))
		budget.chunk = make([]byte, claudeNativePDFScanChunkSize)
	} else {
		err = budget.inflater.(zlib.Resetter).Reset(bytes.NewReader(data), nil)
	}
	if err != nil {
		signals.markOpaque("object_stream_zlib_reset")
		return false
	}
	signals.scan(newClaudePDFStreamLexer(budget, budget.chunk), false, budget)
	if budget.inflatedBytes <= 0 {
		signals.markOpaque("inflate_budget")
	}
	signals.objStmInspected += frame.objStm
	return false
}
