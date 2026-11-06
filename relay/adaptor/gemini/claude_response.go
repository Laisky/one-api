package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/random"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// claudeGeminiToolID generates a Claude-style tool_use id for a Gemini
// function call, which carries no call id of its own.
func claudeGeminiToolID() string {
	return "toolu_" + random.GetRandomString(29)
}

// convertToClaudeResponse converts a buffered Gemini response (JSON or SSE) to
// the Claude Messages format of the original request.
func (a *Adaptor) convertToClaudeResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*http.Response, *model.ErrorWithStatusCode) {
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(errors.WithStack(err), "read_response_body_failed", http.StatusInternalServerError)
	}

	if err = resp.Body.Close(); err != nil {
		return nil, openai.ErrorWrapper(errors.WithStack(err), "close_response_body_failed", http.StatusInternalServerError)
	}

	// Check if it's a streaming response
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return a.convertStreamingToClaudeResponse(c, resp, body, meta)
	}

	// Handle non-streaming response conversion
	return a.convertNonStreamingToClaudeResponse(c, resp, body, meta)
}

// convertNonStreamingToClaudeResponse converts a non-streaming Gemini response
// to a Claude Message. A function-call turn reports stop_reason tool_use (Gemini
// itself reports STOP), and every tool_use block carries an object input.
func (a *Adaptor) convertNonStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte, meta *meta.Meta) (*http.Response, *model.ErrorWithStatusCode) {
	var geminiResp ChatResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		// If it's an error response, pass it through
		newResp := &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		return newResp, nil
	}

	// Convert to Claude Messages format
	claudeResp := model.ClaudeResponse{
		ID:      openai_compatible.ClaudeMessageID(""),
		Type:    "message",
		Role:    "assistant",
		Model:   meta.ActualModelName,
		Content: []model.ClaudeContent{},
	}

	var textBuilder strings.Builder
	var toolArgsBuilder strings.Builder
	imageReceipt := &model.ClaudeImageReceipt{CandidateImageTokens: geminiCandidateImageTokens(geminiResp.UsageMetadata), Incomplete: len(geminiResp.Candidates) > 1 || geminiHasUnrepresentedMedia(&geminiResp)}
	imageCount := 0
	stopReason := ""

	// Convert candidates to content
	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]
		stopReason = mapGeminiFinishReason(candidate.FinishReason)

		// Convert parts to Claude content
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textBuilder.WriteString(part.Text)
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: part.Text})
			}

			if geminiOpenaiCompatible.IsGeminiImageGenerationModel(meta.ActualModelName) {
				if image := claudeGeminiImageText(part); image != "" {
					claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: image})
					imageCount++
				}
			}

			if part.FunctionCall != nil {
				if part.FunctionCall.Arguments != nil {
					if argsBytes, err := json.Marshal(part.FunctionCall.Arguments); err == nil {
						toolArgsBuilder.Write(argsBytes)
					}
				}
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type:  "tool_use",
					ID:    claudeGeminiToolID(),
					Name:  part.FunctionCall.FunctionName,
					Input: openai_compatible.ClaudeToolInputFromArguments(part.FunctionCall.Arguments),
				})
			}
		}
	}
	claudeResp.StopReason = openai_compatible.FinalizeClaudeStopReason(stopReason, openai_compatible.ClaudeContentHasToolUse(claudeResp.Content))

	// Populate usage if metadata available or fall back to local estimates
	if geminiResp.UsageMetadata != nil {
		c.Set(NativeUsageReceiptKey, geminiUsageMetadataToOpenAIUsage(geminiResp.UsageMetadata))
		claudeResp.Usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount
		if geminiResp.UsageMetadata.TotalTokenCount > 0 && geminiResp.UsageMetadata.PromptTokenCount > 0 {
			claudeResp.Usage.OutputTokens = geminiResp.UsageMetadata.TotalTokenCount - geminiResp.UsageMetadata.PromptTokenCount
		} else {
			claudeResp.Usage.OutputTokens = geminiResp.UsageMetadata.CandidatesTokenCount + geminiResp.UsageMetadata.ThoughtsTokenCount
		}
	}

	if claudeResp.Usage.InputTokens == 0 && meta.PromptTokens > 0 {
		claudeResp.Usage.InputTokens = meta.PromptTokens
	}
	if claudeResp.Usage.OutputTokens == 0 {
		completionTokens := openai.CountTokenText(textBuilder.String(), meta.ActualModelName)
		if toolArgs := toolArgsBuilder.String(); toolArgs != "" {
			completionTokens += openai.CountTokenText(toolArgs, meta.ActualModelName)
		}
		claudeResp.Usage.OutputTokens = completionTokens
	}
	// promptTokenCount includes cachedContentTokenCount (OpenAI semantics);
	// settle on that receipt and present it in Anthropic semantics.
	claudeResp.Usage = openai_compatible.PresentConvertedClaudeUsage(c, geminiClaudeUsageReceipt(claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens, geminiReceiptCachedTokens(geminiResp.UsageMetadata)), meta.ActualModelName)

	newResp, failure := openai_compatible.MarshalClaudeHTTPResponse(resp, claudeResp)
	if failure != nil {
		return nil, failure
	}

	if geminiOpenaiCompatible.IsGeminiImageGenerationModel(meta.ActualModelName) {
		bodyLength := int(newResp.ContentLength)
		if length, err := strconv.Atoi(newResp.Header.Get("Content-Length")); err == nil {
			bodyLength = length
		}
		for range imageCount {
			imageReceipt.ImageEndOffsets = append(imageReceipt.ImageEndOffsets, bodyLength)
		}
		c.Set(ctxkey.ClaudeGeminiImageReceipt, imageReceipt)
	}
	return newResp, nil
}

// geminiReceiptCachedTokens returns the cached prompt tokens of a Gemini usage
// receipt. Only a receipt that measured the prompt can attribute a cached
// prefix, mirroring the controller, which applies the native receipt only when
// its prompt count is the settled one. Parameters: usage is the receipt (may be
// nil). Returns: the cached token count, or zero.
func geminiReceiptCachedTokens(usage *UsageMetadata) int {
	if usage == nil || usage.PromptTokenCount <= 0 {
		return 0
	}
	return max(usage.CachedContentTokenCount, 0)
}

// geminiClaudeUsageReceipt builds the OpenAI-semantics receipt a Claude
// conversion settles on. Parameters: prompt and completion are the settled
// counters after estimate fallbacks, and cached is the cached prompt prefix
// already included in prompt. Returns: the canonical usage receipt.
func geminiClaudeUsageReceipt(prompt, completion, cached int) *model.Usage {
	usage := &model.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion}
	if cached > 0 {
		usage.PromptTokensDetails = &model.UsagePromptTokensDetails{CachedTokens: cached}
	}
	return usage
}

// claudeGeminiToolUse is one Gemini function call awaiting rendering.
type claudeGeminiToolUse struct {
	name  string
	input string
}

// convertStreamingToClaudeResponse converts a buffered Gemini SSE stream to a
// conformant Claude Messages SSE stream rendered by the shared emitter: a full
// message_start envelope, text then tool_use blocks with dense indices, one
// message_delta carrying the mapped stop_reason (tool_use for function-call
// turns) and usage, and message_stop. No non-Claude [DONE] sentinel is added.
func (a *Adaptor) convertStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte, meta *meta.Meta) (*http.Response, *model.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)

	lineReader := commonsse.NewLineReader(bytes.NewReader(body), commonsse.DefaultLineBufferSize)

	var textBuilder strings.Builder
	var textBlocks []claudeImageTextBlock
	var pendingText strings.Builder
	imageReceipt := &model.ClaudeImageReceipt{}
	nanoOutput := geminiOpenaiCompatible.IsGeminiImageGenerationModel(meta.ActualModelName)
	var toolUses []claudeGeminiToolUse
	stopReason := ""
	promptTokens := 0
	completionTokens := 0
	cachedTokens := 0

	for {
		line, err := lineReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			imageReceipt.Incomplete = true
			lg.Warn("error reading Gemini stream for Claude conversion", zap.Error(errors.Wrap(err, "read stream")))
			break
		}

		var lineText string
		if line.Oversized {
			payloadBytes, err := io.ReadAll(line.Large)
			if err != nil {
				imageReceipt.Incomplete = true
				lg.Warn("error reading oversized Gemini stream payload", zap.Error(errors.Wrap(err, "read oversized stream")))
				continue
			}
			lineText = "data: " + string(payloadBytes)
		} else {
			lineText = line.Text()
		}

		lineText = strings.TrimSpace(lineText)
		if lineText == "" || strings.HasPrefix(lineText, "event:") || !strings.HasPrefix(lineText, "data:") {
			// Skip blank separators, upstream event annotations, and other fields.
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(lineText, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var geminiResp ChatResponse
		if err := json.Unmarshal([]byte(payload), &geminiResp); err != nil {
			// Handle cases where payload is a quoted JSON string
			unquoted, uerr := strconv.Unquote(payload)
			if uerr != nil || json.Unmarshal([]byte(unquoted), &geminiResp) != nil {
				imageReceipt.Incomplete = true
				lg.Debug("discarding unparseable Gemini stream chunk", zap.String("chunk", payload), zap.Error(err))
				continue
			}
		}

		if geminiResp.UsageMetadata != nil {
			c.Set(NativeUsageReceiptKey, geminiUsageMetadataToOpenAIUsage(geminiResp.UsageMetadata))
			imageReceipt.CandidateImageTokens = geminiCandidateImageTokens(geminiResp.UsageMetadata)
			promptTokens = geminiResp.UsageMetadata.PromptTokenCount
			cachedTokens = geminiReceiptCachedTokens(geminiResp.UsageMetadata)
			completionTokens = geminiResp.UsageMetadata.CandidatesTokenCount + geminiResp.UsageMetadata.ThoughtsTokenCount
		}

		if len(geminiResp.Candidates) == 0 {
			continue
		}

		if len(geminiResp.Candidates) > 1 || geminiHasUnrepresentedMedia(&geminiResp) {
			imageReceipt.Incomplete = true
		}
		candidate := geminiResp.Candidates[0]
		if nanoOutput {
			found := false
			for _, entry := range geminiResp.Candidates {
				if entry.Index != 0 {
					imageReceipt.Incomplete = true
					continue
				}
				if !found {
					candidate = entry
					found = true
				}
			}
			if !found {
				continue
			}
		}
		if candidate.FinishReason != "" {
			stopReason = mapGeminiFinishReason(candidate.FinishReason)
		}

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textBuilder.WriteString(part.Text)
				if nanoOutput {
					pendingText.WriteString(part.Text)
				}
			}
			if nanoOutput {
				if image := claudeGeminiImageText(part); image != "" {
					if pendingText.Len() > 0 {
						textBlocks = append(textBlocks, claudeImageTextBlock{text: pendingText.String()})
						pendingText.Reset()
					}
					textBlocks = append(textBlocks, claudeImageTextBlock{text: image, image: true})
				}
			}

			if part.FunctionCall != nil {
				var argStr string
				if part.FunctionCall.Arguments != nil {
					if bytesValue, err := json.Marshal(part.FunctionCall.Arguments); err == nil {
						argStr = string(bytesValue)
					}
				}
				toolUses = append(toolUses, claudeGeminiToolUse{name: part.FunctionCall.FunctionName, input: argStr})
			}
		}
	}

	if pendingText.Len() > 0 {
		textBlocks = append(textBlocks, claudeImageTextBlock{text: pendingText.String()})
	}
	var toolArgsText strings.Builder
	for _, tool := range toolUses {
		toolArgsText.WriteString(tool.input)
	}

	textOutput := textBuilder.String()
	if !nanoOutput && textOutput != "" {
		textBlocks = append(textBlocks, claudeImageTextBlock{text: textOutput})
	}
	if promptTokens == 0 && meta.PromptTokens > 0 {
		promptTokens = meta.PromptTokens
	}
	if completionTokens == 0 {
		completionTokens = openai.CountTokenText(textOutput, meta.ActualModelName)
		if toolArgs := toolArgsText.String(); toolArgs != "" {
			completionTokens += openai.CountTokenText(toolArgs, meta.ActualModelName)
		}
	}

	// The buffered stream already holds the final receipt, so message_start and
	// the cumulative message_delta both present it in Anthropic semantics.
	claudeUsage := openai_compatible.PresentConvertedClaudeUsage(c, geminiClaudeUsageReceipt(promptTokens, completionTokens, cachedTokens), meta.ActualModelName)
	startUsage := claudeUsage
	startUsage.OutputTokens = 0
	output := &bytes.Buffer{}
	em := openai_compatible.NewClaudeStreamEmitter(output, nil)
	em.Start("", meta.ActualModelName, startUsage)
	for _, block := range textBlocks {
		index := em.StartBlock(model.ClaudeContent{Type: "text"})
		em.TextDelta(index, block.text)
		if block.image {
			imageReceipt.ImageEndOffsets = append(imageReceipt.ImageEndOffsets, output.Len())
		}
		em.StopBlock(index)
	}
	for _, tool := range toolUses {
		index := em.StartBlock(model.ClaudeContent{Type: "tool_use", ID: claudeGeminiToolID(), Name: tool.name})
		em.InputJSONDelta(index, tool.input)
		em.StopBlock(index)
	}
	em.Finish(openai_compatible.FinalizeClaudeStopReason(stopReason, len(toolUses) > 0), claudeUsage)
	if err := em.Err(); err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "render Claude stream"), "marshal_claude_stream_failed", http.StatusInternalServerError)
	}

	outBytes := output.Bytes()
	if nanoOutput {
		c.Set(ctxkey.ClaudeGeminiImageReceipt, imageReceipt)
	}
	newResp := &http.Response{
		StatusCode:    resp.StatusCode,
		Header:        make(http.Header, len(resp.Header)),
		Body:          io.NopCloser(bytes.NewReader(outBytes)),
		ContentLength: int64(len(outBytes)),
	}
	for k, v := range resp.Header {
		newResp.Header[k] = append([]string(nil), v...)
	}
	newResp.Header.Set("Content-Type", "text/event-stream")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(outBytes)))

	return newResp, nil
}
