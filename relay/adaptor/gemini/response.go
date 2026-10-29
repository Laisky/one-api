package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/constant"
	"github.com/Laisky/one-api/relay/model"
)

// ChatResponse represents a Gemini chat generation response.
type ChatResponse struct {
	Candidates     []ChatCandidate    `json:"candidates"`
	PromptFeedback ChatPromptFeedback `json:"promptFeedback"`
	UsageMetadata  *UsageMetadata     `json:"usageMetadata,omitempty"`
	ModelVersion   string             `json:"modelVersion,omitempty"`
	ResponseId     string             `json:"responseId,omitempty"`
}

// GetResponseText returns the first text part from the first Gemini candidate, or an empty string.
func (g *ChatResponse) GetResponseText() string {
	if g == nil {
		return ""
	}
	if len(g.Candidates) > 0 && len(g.Candidates[0].Content.Parts) > 0 {
		return g.Candidates[0].Content.Parts[0].Text
	}
	return ""
}

// ChatCandidate represents one candidate returned by Gemini.
type ChatCandidate struct {
	Content       ChatContent        `json:"content"`
	FinishReason  string             `json:"finishReason"`
	Index         int64              `json:"index"`
	SafetyRatings []ChatSafetyRating `json:"safetyRatings"`
}

// ChatSafetyRating represents Gemini safety metadata for a response or prompt.
type ChatSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// ChatPromptFeedback represents Gemini prompt-level safety feedback.
type ChatPromptFeedback struct {
	SafetyRatings []ChatSafetyRating `json:"safetyRatings"`
}

// getToolCalls extracts function call tool information from a Gemini chat candidate.
// It processes all parts that contain function calls and returns them as OpenAI-compatible tool calls.
// Returns an empty slice if no function calls are present or if the candidate has no parts.
func getToolCalls(c *gin.Context, candidate *ChatCandidate) []model.Tool {
	lg := gmw.GetLogger(c)
	var toolCalls []model.Tool

	// Guard against empty Parts slice to prevent index out of range panic
	if len(candidate.Content.Parts) == 0 {
		lg.Debug("getToolCalls: candidate has no parts, returning empty tool calls")
		return toolCalls
	}

	// Process all parts that contain function calls, not just the first one
	for _, part := range candidate.Content.Parts {
		if part.FunctionCall == nil {
			continue
		}
		argsBytes, err := json.Marshal(part.FunctionCall.Arguments)
		if err != nil {
			lg.Error("getToolCalls: failed to marshal function call arguments",
				zap.String("function_name", part.FunctionCall.FunctionName),
				zap.Error(err))
			continue
		}
		toolCall := model.Tool{
			Id:   fmt.Sprintf("call_%s", random.GetUUID()),
			Type: "function",
			Function: &model.Function{
				Arguments: string(argsBytes),
				Name:      part.FunctionCall.FunctionName,
			},
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls
}

// getStreamingToolCalls creates tool calls for streaming responses with Index field set.
// It processes all parts that contain function calls and returns them with proper indexing
// for stream delta accumulation.
func getStreamingToolCalls(c *gin.Context, candidate *ChatCandidate) []model.Tool {
	lg := gmw.GetLogger(c)
	var toolCalls []model.Tool

	// Process all parts in case there are multiple function calls
	for partIndex, part := range candidate.Content.Parts {
		if part.FunctionCall == nil {
			continue
		}
		argsBytes, err := json.Marshal(part.FunctionCall.Arguments)
		if err != nil {
			lg.Error("getStreamingToolCalls: failed to marshal function call arguments",
				zap.String("function_name", part.FunctionCall.FunctionName),
				zap.Error(err))
			continue
		}
		// Set index for streaming tool calls - use the part index to ensure proper ordering
		// This handles the case where Gemini might support multiple parallel tool calls in the future
		index := partIndex
		toolCall := model.Tool{
			Id:   fmt.Sprintf("call_%s", random.GetUUID()),
			Type: "function",
			Function: &model.Function{
				Arguments: string(argsBytes),
				Name:      part.FunctionCall.FunctionName,
			},
			Index: &index, // Set index for streaming delta accumulation
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls
}

// responseGeminiChat2OpenAI converts a Gemini chat response into an OpenAI-compatible text response.
// Parameters: c is the current request context and response is the parsed Gemini response.
// Returns: an OpenAI-compatible text response.
func responseGeminiChat2OpenAI(c *gin.Context, response *ChatResponse) *openai.TextResponse {
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: make([]openai.TextResponseChoice, 0, len(response.Candidates)),
	}
	for i, candidate := range response.Candidates {
		choice := openai.TextResponseChoice{
			Index: i,
			Message: model.Message{
				Role: "assistant",
			},
			FinishReason: constant.StopFinishReason,
		}

		toolCalls := getToolCalls(c, &candidate)
		if len(toolCalls) > 0 {
			choice.Message.ToolCalls = toolCalls
		}

		if len(candidate.Content.Parts) > 0 {
			var textParts []string
			var structured []model.MessageContent

			for _, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					continue
				}

				if part.Text != "" {
					textParts = append(textParts, part.Text)
					structured = append(structured, model.MessageContent{
						Type: model.ContentTypeText,
						Text: &part.Text,
					})
				}

				if part.InlineData != nil && part.InlineData.Data != "" && part.InlineData.MimeType != "" &&
					isGeminiImageMimeType(part.InlineData.MimeType) {
					imageURL := &model.ImageURL{
						Url: fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data),
					}
					structured = append(structured, model.MessageContent{
						Type:     model.ContentTypeImageURL,
						ImageURL: imageURL,
					})
				}
				if part.FileData != nil && part.FileData.FileURI != "" && isGeminiImageMimeType(part.FileData.MimeType) {
					imageURL := &model.ImageURL{
						Url: part.FileData.FileURI,
					}
					structured = append(structured, model.MessageContent{
						Type:     model.ContentTypeImageURL,
						ImageURL: imageURL,
					})
				}
			}

			joined := strings.Join(textParts, "\n")
			if len(structured) > 1 || (len(structured) == 1 && structured[0].Type != model.ContentTypeText) {
				choice.Message.Content = structured
			} else if joined != "" {
				choice.Message.Content = joined
			} else if len(toolCalls) == 0 {
				choice.Message.Content = ""
			}
		} else {
			choice.Message.Content = ""
			choice.FinishReason = candidate.FinishReason
		}

		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

// streamResponseGeminiChat2OpenAI converts a Gemini stream chunk into an OpenAI-compatible stream chunk.
// Parameters: c is the current request context and geminiResponse is the parsed Gemini stream chunk.
// Returns: an OpenAI-compatible stream response, or nil when the chunk has no usable candidate content.
func streamResponseGeminiChat2OpenAI(c *gin.Context, geminiResponse *ChatResponse) *openai.ChatCompletionsStreamResponse {
	if len(geminiResponse.Candidates) == 0 {
		return nil
	}
	choices := make([]openai.ChatCompletionsStreamResponseChoice, 0, len(geminiResponse.Candidates))
	for _, candidate := range geminiResponse.Candidates {
		if len(candidate.Content.Parts) == 0 {
			continue
		}
		var choice openai.ChatCompletionsStreamResponseChoice
		choice.Index = int(candidate.Index)
		choice.Delta.Role = "assistant"
		var content []model.MessageContent
		var textParts []string
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text := part.Text
				textParts = append(textParts, text)
				content = append(content, model.MessageContent{Type: model.ContentTypeText, Text: &text})
			}
			imageURL := ""
			if part.InlineData != nil && part.InlineData.Data != "" && part.InlineData.MimeType != "" && isGeminiImageMimeType(part.InlineData.MimeType) {
				imageURL = fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)
			} else if part.FileData != nil && part.FileData.FileURI != "" && isGeminiImageMimeType(part.FileData.MimeType) {
				imageURL = part.FileData.FileURI
			}
			if imageURL != "" {
				content = append(content, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: imageURL}})
			}
		}
		if len(content) > len(textParts) {
			choice.Delta.Content = content
		} else if len(textParts) > 0 {
			choice.Delta.Content = strings.Join(textParts, "")
		}
		choice.Delta.ToolCalls = getStreamingToolCalls(c, &candidate)

		choices = append(choices, choice)
	}
	if len(choices) == 0 {
		return nil
	}
	var response openai.ChatCompletionsStreamResponse
	response.Id = tracing.GenerateChatCompletionID(c)
	response.Created = helper.GetTimestamp()
	response.Object = "chat.completion.chunk"
	response.Model = "gemini"
	response.Choices = choices

	return &response
}

// embeddingResponseGemini2OpenAI converts Gemini embedding results into OpenAI format.
// Parameters: response is the parsed Gemini embedding response, promptTokens is the locally counted input token total, and details is the optional modality breakdown from preflight countTokens.
// Returns: an OpenAI-compatible embedding response populated with a billing-safe usage fallback.
func embeddingResponseGemini2OpenAI(response *EmbeddingResponse, promptTokens int, details *model.UsagePromptTokensDetails) *openai.EmbeddingResponse {
	openAIEmbeddingResponse := openai.EmbeddingResponse{
		Object: "list",
		Data:   make([]openai.EmbeddingResponseItem, 0, len(response.Embeddings)),
		Model:  "gemini-embedding",
		Usage: model.Usage{
			PromptTokens:        promptTokens,
			TotalTokens:         promptTokens,
			PromptTokensDetails: details,
		},
	}
	for _, item := range response.Embeddings {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, openai.EmbeddingResponseItem{
			Object:    `embedding`,
			Index:     0,
			Embedding: item.Values,
		})
	}
	return &openAIEmbeddingResponse
}

// embeddingPromptTokensDetailsFromContext returns preflight embedding modality details stored in the request context.
// Parameters: c is the current request context.
// Returns: the stored prompt token details or nil when no preflight details were captured.
func embeddingPromptTokensDetailsFromContext(c *gin.Context) *model.UsagePromptTokensDetails {
	if c == nil {
		return nil
	}
	raw, exists := c.Get(ctxkey.EmbeddingPromptTokensDetails)
	if !exists {
		return nil
	}

	details, ok := raw.(*model.UsagePromptTokensDetails)
	if !ok {
		return nil
	}
	return details
}

// geminiOutputImageCounts aggregates image counts for Gemini output parts.
type geminiOutputImageCounts struct {
	Total  int
	Inline int
	File   int
}

// isGeminiImageMimeType reports whether the MIME type should be treated as an image.
// Parameters: mimeType is the MIME type string from Gemini output.
// Returns: true when the MIME type is empty or starts with "image/".
func isGeminiImageMimeType(mimeType string) bool {
	if mimeType == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(mimeType), "image/")
}

// countGeminiOutputImages counts image parts (inline data and file references) in a Gemini chat response.
// Parameters: response is the parsed Gemini ChatResponse payload.
// Returns: aggregated image counts by representation and total.
func countGeminiOutputImages(response *ChatResponse) geminiOutputImageCounts {
	if response == nil {
		return geminiOutputImageCounts{}
	}
	var counts geminiOutputImageCounts
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				continue
			}
			if part.InlineData != nil && part.InlineData.Data != "" && part.InlineData.MimeType != "" && isGeminiImageMimeType(part.InlineData.MimeType) {
				counts.Inline++
				counts.Total++
			}
			if part.FileData != nil && part.FileData.FileURI != "" && isGeminiImageMimeType(part.FileData.MimeType) {
				counts.File++
				counts.Total++
			}
		}
	}
	return counts
}

// countGeminiStreamImages counts image representations selected by the stream converter.
func countGeminiStreamImages(response *ChatResponse) geminiOutputImageCounts {
	var counts geminiOutputImageCounts
	if response == nil || len(response.Candidates) == 0 {
		return counts
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" && part.InlineData.MimeType != "" && isGeminiImageMimeType(part.InlineData.MimeType) {
				counts.Inline++
				counts.Total++
			} else if part.FileData != nil && part.FileData.FileURI != "" && isGeminiImageMimeType(part.FileData.MimeType) {
				counts.File++
				counts.Total++
			}
		}
	}
	return counts
}

// geminiHasUnrepresentedMedia reports output media for which this gateway has
// no complete downstream representation. Gemini's IMAGE-token receipt is
// aggregate, not per candidate or per image. If any media part is dropped or
// ambiguous, excluding the entire IMAGE-token bucket after billing only the
// visible images could undercharge. Retain conservative token pricing instead.
func geminiHasUnrepresentedMedia(response *ChatResponse) bool {
	if response == nil {
		return false
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				if part.FunctionCall != nil || part.FileData != nil ||
					part.InlineData.Data == "" || part.InlineData.MimeType == "" ||
					!isGeminiImageMimeType(part.InlineData.MimeType) {
					return true
				}
			}
			if part.FileData != nil {
				if part.FunctionCall != nil || part.FileData.FileURI == "" ||
					part.FileData.MimeType == "" || !isGeminiImageMimeType(part.FileData.MimeType) {
					return true
				}
			}
		}
	}
	return false
}

// recordGeminiOutputImageCount accumulates output image counts in the Gin context.
// Parameters: c is the Gin context for the request; count is the number of images to add.
// Returns: nothing; the aggregated count is stored under ctxkey.OutputImageCount.
func recordGeminiOutputImageCount(c *gin.Context, count int) {
	if c == nil || count <= 0 {
		return
	}
	if raw, ok := c.Get(ctxkey.OutputImageCount); ok {
		if existing, ok := raw.(int); ok {
			c.Set(ctxkey.OutputImageCount, existing+count)
			return
		}
	}
	c.Set(ctxkey.OutputImageCount, count)
}

// geminiUsageMetadataToOpenAIUsage maps Gemini's authoritative usageMetadata into model.Usage.
// Parameters: meta is the Gemini usage metadata from a streaming or non-streaming response.
// Returns: an OpenAI-compatible usage snapshot, or nil when meta is nil.
func geminiUsageMetadataToOpenAIUsage(meta *UsageMetadata) *model.Usage {
	if meta == nil {
		return nil
	}

	usage := &model.Usage{
		PromptTokens:         meta.PromptTokenCount,
		CompletionTokens:     meta.CandidatesTokenCount + meta.ThoughtsTokenCount,
		TotalTokens:          meta.TotalTokenCount,
		CandidateImageTokens: geminiCandidateImageTokens(meta),
	}
	if meta.CachedContentTokenCount > 0 {
		usage.PromptTokensDetails = &model.UsagePromptTokensDetails{
			CachedTokens: meta.CachedContentTokenCount,
		}
	}
	if meta.ThoughtsTokenCount > 0 {
		usage.CompletionTokensDetails = &model.UsageCompletionTokensDetails{
			ReasoningTokens: meta.ThoughtsTokenCount,
		}
	}
	return usage
}

// geminiCandidateImageTokens extracts only provider-reported IMAGE output tokens.
// Parameters: meta is Gemini's usage receipt. Returns: a nonnegative count
// bounded by candidatesTokenCount, not an estimated count per image.
func geminiCandidateImageTokens(meta *UsageMetadata) int {
	if meta == nil || meta.CandidatesTokenCount <= 0 {
		return 0
	}
	count := 0
	for _, entry := range meta.CandidatesTokensDetails {
		if !strings.EqualFold(entry.Modality, "IMAGE") || entry.TokenCount <= 0 {
			continue
		}
		// Bound cumulative counts without overflow, even for malformed receipts.
		count += min(entry.TokenCount, meta.CandidatesTokenCount-count)
		if count == meta.CandidatesTokenCount {
			break
		}
	}
	return count
}

// geminiStreamText extracts textual stream content for fallback token estimation.
func geminiStreamText(message model.Message) string {
	var text strings.Builder
	for _, part := range message.ParseContent() {
		if part.Type == model.ContentTypeText && part.Text != nil {
			text.WriteString(*part.Text)
		}
	}
	return text.String()
}
