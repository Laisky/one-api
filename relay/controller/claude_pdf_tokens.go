package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"math"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
)

// claudeNativePDFPageTokens converts the summed native PDF pages of one request
// into an admission estimate: pages are capped at the documented per-request
// page limit, multiplied by the trusted per-page estimate, and capped at the
// largest documented context window because a larger input is rejected by the
// provider rather than billed. It returns an error for invalid inputs.
func claudeNativePDFPageTokens(pages, tokensPerPage int) (int, error) {
	if pages < 0 || tokensPerPage < 1 || tokensPerPage > math.MaxInt/claudeNativePDFMaxPagesPerRequest {
		return 0, errors.New("invalid native PDF page count or per-page estimate")
	}
	pages = min(pages, claudeNativePDFMaxPagesPerRequest)
	return min(pages*tokensPerPage, claudeNativePDFContextTokenCeiling), nil
}

// claudeNativePDFPages validates one native base64 PDF source and returns its
// conservative page estimate. Documents larger than the in-memory scan limit,
// and documents whose structure hides page objects, count as the per-request
// page limit. The provider payload is never modified. skipScan validates the
// source without scanning once the request already reached the page limit.
func claudeNativePDFPages(ctx context.Context, data string, budget *claudePDFScanBudget, skipScan bool) (int, error) {
	limit := claudeNativePDFScanMaxDecodedBytes
	if skipScan {
		limit = 0
	}
	decoded, size, err := claudePDFDecode(data, limit)
	if err != nil {
		return 0, err
	}
	if skipScan {
		return claudeNativePDFMaxPagesPerRequest, nil
	}
	signals := claudePDFPageSignals{}
	if decoded == nil {
		signals.markOpaque("decoded_size_limit")
	} else {
		signals = scanClaudePDFPages(decoded, budget)
	}
	pages := signals.pages()
	lg := gmw.GetLogger(ctx)
	lg.Debug("estimated native PDF pages for admission",
		zap.Int("decoded_bytes", size),
		zap.Int("pages", pages),
		zap.Int("page_names", signals.pageNames),
		zap.Int("page_tree_count", signals.pageTreeCount),
		zap.Int("kids_entries", signals.kidsEntries),
		zap.Int("object_streams", signals.objStmNames),
		zap.String("opaque_reason", signals.opaqueReason),
	)
	return pages, nil
}

// claudePDFDecode validates base64 PDF data and returns its decoded size. The
// decoded bytes are returned only when they fit within limit; larger documents
// are validated in streaming mode and returned as nil. ASCII transport
// whitespace is ignored; malformed or empty data returns an error.
func claudePDFDecode(data string, limit int) ([]byte, int, error) {
	encoded := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
		default:
			encoded++
		}
	}
	decoder := base64.NewDecoder(base64.StdEncoding, &claudePDFBase64Reader{data: data})
	if upper := encoded / 4 * 3; upper > limit {
		size, err := io.Copy(io.Discard, decoder)
		if err != nil {
			return nil, 0, errors.Wrap(err, "decode native PDF source for quota estimate")
		}
		if size == 0 || size > int64(math.MaxInt) {
			return nil, 0, errors.New("native PDF source has empty or unrepresentable decoded size")
		}
		// Padding can leave a document at most two bytes under the encoded
		// bound; it is still reported without bytes, which callers treat
		// conservatively as an unscanned document.
		return nil, int(size), nil
	}
	var decoded bytes.Buffer
	decoded.Grow(encoded / 4 * 3)
	if _, err := io.Copy(&decoded, decoder); err != nil {
		return nil, 0, errors.Wrap(err, "decode native PDF source for quota estimate")
	}
	if decoded.Len() == 0 {
		return nil, 0, errors.New("native PDF source has empty decoded size")
	}
	return decoded.Bytes(), decoded.Len(), nil
}

// claudePDFBase64Reader streams an existing base64 string while skipping ASCII
// transport whitespace, without copying or retaining a second document buffer.
type claudePDFBase64Reader struct {
	data   string
	offset int
}

// Read fills buffer with non-whitespace encoded bytes and returns EOF when all
// input has been consumed. It preserves every non-whitespace byte for decoding.
func (reader *claudePDFBase64Reader) Read(buffer []byte) (int, error) {
	written := 0
	for reader.offset < len(reader.data) && written < len(buffer) {
		value := reader.data[reader.offset]
		reader.offset++
		switch value {
		case ' ', '\t', '\r', '\n':
			continue
		}
		buffer[written] = value
		written++
	}
	if written == 0 && reader.offset == len(reader.data) {
		return 0, io.EOF
	}
	return written, nil
}
