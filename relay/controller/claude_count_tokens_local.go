package controller

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
)

// estimateClaudeCountTokens answers count_tokens locally with the same prompt
// estimator the Messages endpoint uses for admission (messages, system, tools,
// images and documents). Inputs the native counter rejects are rejected first,
// which also guarantees the estimate never fetches a remote image.
//
// Parameters:
//   - c: the request context.
//   - request: the decoded request with the mapped model.
//   - raw: the raw request body.
//
// Return values:
//   - int: the estimated input tokens.
//   - *claudeCountTokensFailure: a 400 failure, or nil.
func estimateClaudeCountTokens(c *gin.Context, request *ClaudeMessagesRequest, raw []byte) (int, *claudeCountTokensFailure) {
	if err := validateClaudeCountTokensLocalInput(request, raw); err != nil {
		return 0, &claudeCountTokensFailure{status: http.StatusBadRequest, err: errors.WithStack(err)}
	}
	tokens, err := getClaudeMessagesPromptTokens(gmw.Ctx(c), request)
	if err != nil {
		return 0, &claudeCountTokensFailure{status: http.StatusBadRequest, err: errors.Wrap(err, "estimate count_tokens input")}
	}
	return tokens, nil
}

// validateClaudeCountTokensLocalInput mirrors the inputs Anthropic's
// count_tokens rejects with 400: server tools (an unrecognized type such as
// the advisor tool is counted, not rejected), the MCP connector, code
// execution containers, and url/file image or document sources.
//
// Parameters:
//   - request: the decoded request.
//   - raw: the raw request body, scanned with exact keys for capabilities.
//
// Return values:
//   - error: the first unsupported input, or nil.
func validateClaudeCountTokensLocalInput(request *ClaudeMessagesRequest, raw []byte) error {
	uses, unknown, err := collectClaudeServerCapabilities(raw)
	if err != nil {
		return errors.Wrap(err, "invalid tool declaration")
	}
	var rejected []string
	for name := range uses {
		if !slices.Contains(unknown, name) {
			rejected = append(rejected, name)
		}
	}
	if len(rejected) > 0 {
		slices.Sort(rejected)
		return errors.Errorf("count_tokens does not support server tools, the MCP connector or containers: %s", strings.Join(rejected, ", "))
	}

	if err := validateClaudeCountTokensSources(request.System, "system"); err != nil {
		return errors.WithStack(err)
	}
	for i, message := range request.Messages {
		if err := validateClaudeCountTokensSources(message.Content, "messages["+strconv.Itoa(i)+"].content"); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// claudeCountTokensBlocks is one pending content array of the source walk.
type claudeCountTokensBlocks struct {
	// location names the array for error messages.
	location string
	// blocks are the content blocks still to inspect.
	blocks []any
}

// validateClaudeCountTokensSources walks content iteratively, including
// tool_result, search_result and content-document nesting, and rejects every
// image whose source is not inline base64 data and every url or file document.
//
// Parameters:
//   - content: a system prompt or message content (string or block array).
//   - location: the JSON location of content, for error messages.
//
// Return values:
//   - error: the first unsupported source, or nil.
func validateClaudeCountTokensSources(content any, location string) error {
	root, ok := content.([]any)
	if !ok {
		return nil
	}
	stack := []claudeCountTokensBlocks{{location: location, blocks: root}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for i, item := range current.blocks {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			at := current.location + "[" + strconv.Itoa(i) + "]"
			source, _ := block["source"].(map[string]any)
			switch block["type"] {
			case "image":
				if !claudeImageSourceIsInlineBase64(source) {
					return errors.Errorf("%s: count_tokens only accepts base64 image sources", at)
				}
			case "document":
				if claudeDocumentSourceIsReference(source) {
					return errors.Errorf("%s: count_tokens does not accept url or file document sources", at)
				}
				if nested, ok := source["content"].([]any); ok {
					stack = append(stack, claudeCountTokensBlocks{location: at + ".source.content", blocks: nested})
				}
			case "tool_result", "search_result":
				if nested, ok := block["content"].([]any); ok {
					stack = append(stack, claudeCountTokensBlocks{location: at + ".content", blocks: nested})
				}
			}
		}
	}
	return nil
}

// claudeImageSourceIsInlineBase64 reports whether an image source carries
// inline base64 data, the only image source the counters can measure without
// fetching anything.
//
// Parameters:
//   - source: the image block's source object, or nil.
//
// Return values:
//   - bool: true for {"type":"base64","data":"<non-empty>"}.
func claudeImageSourceIsInlineBase64(source map[string]any) bool {
	if source == nil {
		return false
	}
	kind, _ := source["type"].(string)
	data, _ := source["data"].(string)
	return kind == "base64" && data != ""
}

// claudeDocumentSourceIsReference reports whether a document source points at
// remote or uploaded bytes (url or file) instead of carrying them inline.
//
// Parameters:
//   - source: the document block's source object, or nil.
//
// Return values:
//   - bool: true for url and file sources, including untyped references.
func claudeDocumentSourceIsReference(source map[string]any) bool {
	if source == nil {
		return false
	}
	switch kind, _ := source["type"].(string); kind {
	case "url", "file":
		return true
	case "":
		_, hasURL := source["url"]
		_, hasFileID := source["file_id"]
		return hasURL || hasFileID
	default:
		return false
	}
}
