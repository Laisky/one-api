package controller

import (
	"bytes"
	"encoding/base64"
	"strings"

	"github.com/Laisky/errors/v2"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// OCR admission policy. A layout_parsing request is quoted before dispatch from
// its validated document selection; the quote is a reservation, not a ceiling,
// because settlement always bills the provider's measured receipt.
const (
	// ocrMaxDocumentPages is the largest document a provider accepts per call.
	// BigModel documents 100 pages for GLM-OCR
	// (https://docs.bigmodel.cn/cn/guide/models/vlm/glm-ocr.md); Z.ai's API
	// reference says 30 while its guide says 100
	// (https://docs.z.ai/api-reference/tools/layout-parsing.md,
	// https://docs.z.ai/guides/vlm/glm-ocr), so the larger value is the
	// conservative bound when the page count cannot be known before dispatch.
	ocrMaxDocumentPages = 100
	// ocrAllowanceInputTokensPerPage reserves half of GLM-OCR's 32K context for
	// each page image. BigModel estimates about 2,000 A4 pages per CNY at 0.2 CNY
	// per 1M tokens, i.e. roughly 2,500 tokens per page in total, so this leaves
	// more than six times the documented average.
	ocrAllowanceInputTokensPerPage = 16_384
	// ocrAllowanceOutputTokensPerPage matches GLM-OCR's 4,096-token output limit.
	ocrAllowanceOutputTokensPerPage = 4_096
	// ocrInlineSniffChars is the base64 prefix decoded to identify an inline image.
	ocrInlineSniffChars = 16
)

// OCR allowance sources recorded in the consume log metadata.
const (
	ocrAllowanceFromPageRange   = "page_range"
	ocrAllowanceFromInlineImage = "inline_image"
	ocrAllowanceFromMaxPages    = "document_page_limit"
)

// ocrAllowance is the validated, conservative workload admitted before dispatch.
type ocrAllowance struct {
	pages         int
	explicitRange bool
	source        string
	inputTokens   int
	outputTokens  int
}

// quoteOCRAllowance validates the page selection of request and returns the
// conservative workload to reserve. An explicit inclusive range is used as-is
// (capped at the provider document limit), a single inline PNG/JPEG counts as
// one page, and any other document reserves the documented page limit because
// remote content is never fetched to count its pages.
// Parameters: request is the decoded OCR request. Returns: the allowance or a
// validation error for a negative or reversed page range.
func quoteOCRAllowance(request *relaymodel.OCRRequest) (ocrAllowance, error) {
	if request == nil {
		return ocrAllowance{}, errors.New("OCR request is nil")
	}
	start, end := request.StartPageID, request.EndPageID
	if (start != nil && *start < 0) || (end != nil && *end < 0) {
		return ocrAllowance{}, errors.New("start_page_id and end_page_id must be nonnegative")
	}
	if start != nil && end != nil && *end < *start {
		return ocrAllowance{}, errors.New("end_page_id must not be less than start_page_id")
	}

	allowance := ocrAllowance{pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages}
	switch {
	case start != nil && end != nil:
		allowance.pages = min(*end-*start+1, ocrMaxDocumentPages)
		allowance.explicitRange = true
		allowance.source = ocrAllowanceFromPageRange
	case isInlineOCRImage(request.File):
		allowance.pages = 1
		allowance.source = ocrAllowanceFromInlineImage
	}
	allowance.inputTokens = allowance.pages * ocrAllowanceInputTokensPerPage
	allowance.outputTokens = allowance.pages * ocrAllowanceOutputTokensPerPage
	return allowance, nil
}

// isInlineOCRImage reports whether file is inline base64 content whose decoded
// bytes start with a PNG or JPEG signature. A data URI must also declare an
// image media type. Remote URLs and PDFs are never treated as single images.
// Parameters: file is the request's file field. Returns: true for one inline image.
func isInlineOCRImage(file string) bool {
	payload := strings.TrimSpace(file)
	if strings.HasPrefix(payload, "data:") {
		header, data, ok := strings.Cut(payload, ",")
		if !ok {
			return false
		}
		mediaType := strings.ToLower(strings.TrimPrefix(header, "data:"))
		if !strings.HasSuffix(mediaType, ";base64") ||
			!(strings.HasPrefix(mediaType, "image/png") || strings.HasPrefix(mediaType, "image/jpeg") || strings.HasPrefix(mediaType, "image/jpg")) {
			return false
		}
		payload = data
	} else if strings.Contains(payload, "://") {
		return false
	}
	prefix := payload[:min(len(payload), ocrInlineSniffChars)]
	prefix = prefix[:len(prefix)-len(prefix)%4]
	decoded, err := base64.StdEncoding.DecodeString(prefix)
	if err != nil {
		return false
	}
	return bytes.HasPrefix(decoded, []byte("\x89PNG\r\n\x1a\n")) || bytes.HasPrefix(decoded, []byte{0xFF, 0xD8, 0xFF})
}
