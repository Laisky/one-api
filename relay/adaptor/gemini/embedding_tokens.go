package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// EmbeddingCountTokensInvoker executes a provider-specific countTokens request for normalized embedding contents.
// Parameters: ctx carries request-scoped cancellation, and contents is the normalized Gemini content list.
// Returns: the upstream countTokens response or a provider-shaped API error.
type EmbeddingCountTokensInvoker func(ctx context.Context, contents []ChatContent) (*CountTokensResponse, *relaymodel.ErrorWithStatusCode)

// BuildEmbeddingContents normalizes an OpenAI-compatible embeddings input into Gemini contents.
// Parameters: input is the original request input value and may contain strings, part objects, or content objects.
// Returns: the normalized contents, whether any non-text modality is present, and an error when the payload shape is unsupported.
func BuildEmbeddingContents(input any) ([]ChatContent, bool, error) {
	if input == nil {
		return nil, false, errors.New("embedding input is required")
	}

	switch value := input.(type) {
	case string:
		content := ChatContent{Parts: []Part{{Text: value}}}
		return []ChatContent{content}, false, nil
	case []string:
		if len(value) == 0 {
			return nil, false, errors.New("embedding input must not be empty")
		}
		contents := make([]ChatContent, 0, len(value))
		for _, item := range value {
			contents = append(contents, ChatContent{Parts: []Part{{Text: item}}})
		}
		return contents, false, nil
	case map[string]any:
		content, multimodal, err := buildEmbeddingContentFromMap(value)
		if err != nil {
			return nil, false, errors.Wrap(err, "build embedding content from map")
		}
		return []ChatContent{content}, multimodal, nil
	case []any:
		if len(value) == 0 {
			return nil, false, errors.New("embedding input must not be empty")
		}

		contents := make([]ChatContent, 0, len(value))
		hasMultimodal := false
		for _, item := range value {
			content, multimodal, err := buildEmbeddingContentFromItem(item)
			if err != nil {
				return nil, false, errors.Wrap(err, "build embedding content from item")
			}
			if multimodal {
				hasMultimodal = true
			}
			contents = append(contents, content)
		}
		return contents, hasMultimodal, nil
	default:
		return nil, false, errors.Errorf("unsupported embedding input type %T", input)
	}
}

// EstimateEmbeddingPromptUsageWithCounter counts Gemini embedding prompt usage with a provider-specific counter.
// Parameters: ctx carries cancellation, request is the incoming embeddings payload, and invoker executes countTokens upstream.
// Returns: the prompt usage snapshot, whether the request is multimodal, and an API error when safe preflight estimation is not possible.
func EstimateEmbeddingPromptUsageWithCounter(ctx context.Context, request *relaymodel.GeneralOpenAIRequest, invoker EmbeddingCountTokensInvoker) (*relaymodel.Usage, bool, *relaymodel.ErrorWithStatusCode) {
	if request == nil {
		return nil, false, openai.ErrorWrapper(errors.New("request is nil"), "invalid_embedding_input", http.StatusBadRequest)
	}

	contents, hasMultimodal, err := BuildEmbeddingContents(request.Input)
	if err != nil {
		return nil, false, openai.ErrorWrapper(errors.Wrap(err, "build_embedding_contents"), "invalid_embedding_input", http.StatusBadRequest)
	}

	countTokensResp, bizErr := invoker(ctx, contents)
	if bizErr != nil {
		if !hasMultimodal {
			return fallbackTextEmbeddingUsage(contents, request.Model), false, nil
		}
		return nil, true, bizErr
	}

	usage := buildPromptUsageFromCountTokens(countTokensResp)
	if hasMultimodal && !hasMultimodalPromptTokenDetails(usage.PromptTokensDetails) {
		return nil, true, openai.ErrorWrapper(errors.New("countTokens response missing multimodal token details"), "count_tokens_missing_multimodal_details", http.StatusBadGateway)
	}
	if usage.PromptTokens <= 0 {
		if !hasMultimodal {
			return fallbackTextEmbeddingUsage(contents, request.Model), false, nil
		}
		return nil, true, openai.ErrorWrapper(errors.New("countTokens returned zero prompt tokens"), "count_tokens_zero_prompt_tokens", http.StatusBadGateway)
	}

	return usage, hasMultimodal, nil
}

// EstimateEmbeddingPromptUsage counts Gemini API embedding prompt usage via models.countTokens.
// Parameters: c is the request context, meta contains channel routing data, and request is the embeddings payload.
// Returns: the prompt usage snapshot, whether the request is multimodal, and an API error when the estimate cannot be produced safely.
func EstimateEmbeddingPromptUsage(c *gin.Context, meta *meta.Meta, request *relaymodel.GeneralOpenAIRequest) (*relaymodel.Usage, bool, *relaymodel.ErrorWithStatusCode) {
	return EstimateEmbeddingPromptUsageWithCounter(gmw.Ctx(c), request, func(ctx context.Context, contents []ChatContent) (*CountTokensResponse, *relaymodel.ErrorWithStatusCode) {
		return countEmbeddingTokens(ctx, meta, contents)
	})
}

// countEmbeddingTokens calls Gemini's models.countTokens endpoint for embedding contents.
// Parameters: ctx carries cancellation, meta contains provider connection metadata, and contents is the normalized Gemini content list.
// Returns: the parsed countTokens response or a Gemini-shaped API error.
func countEmbeddingTokens(ctx context.Context, meta *meta.Meta, contents []ChatContent) (*CountTokensResponse, *relaymodel.ErrorWithStatusCode) {
	requestBody, err := json.Marshal(struct {
		Contents []ChatContent `json:"contents"`
	}{
		Contents: contents,
	})
	if err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "marshal_count_tokens_request"), "count_tokens_request_marshal_failed", http.StatusInternalServerError)
	}

	countTokensURL := buildCountTokensURL(meta)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, countTokensURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "new_count_tokens_request"), "count_tokens_request_build_failed", http.StatusInternalServerError)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", meta.APIKey)

	httpClient := client.HTTPClient
	if httpClient == nil {
		client.Init()
		httpClient = client.HTTPClient
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, geminiCountTokensError(http.StatusBadGateway, errors.Wrap(err, "perform_count_tokens_request"))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, geminiCountTokensError(http.StatusBadGateway, errors.Wrap(err, "read_count_tokens_response"))
	}

	var countTokensResp CountTokensResponse
	if err = json.Unmarshal(responseBody, &countTokensResp); err != nil {
		return nil, geminiCountTokensError(http.StatusBadGateway, errors.Wrap(err, "unmarshal_count_tokens_response"))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, geminiCountTokensAPIError(resp.StatusCode, countTokensResp.Error, responseBody)
	}
	if countTokensResp.Error != nil {
		return nil, geminiCountTokensAPIError(http.StatusBadGateway, countTokensResp.Error, responseBody)
	}

	return &countTokensResp, nil
}

// buildCountTokensURL constructs the Gemini countTokens endpoint for the selected model.
// Parameters: meta contains the provider base URL, configured API version, and target model name.
// Returns: the absolute countTokens endpoint URL.
func buildCountTokensURL(meta *meta.Meta) string {
	version := resolveGeminiAPIVersion(meta.ActualModelName, meta.Config.APIVersion)
	return meta.BaseURL + "/" + version + "/models/" + meta.ActualModelName + ":countTokens"
}

// geminiCountTokensError converts a transport or parsing failure into a Gemini-style API error.
// Parameters: statusCode is the response status we want to surface and err is the underlying failure.
// Returns: a populated API error wrapper.
func geminiCountTokensError(statusCode int, err error) *relaymodel.ErrorWithStatusCode {
	return &relaymodel.ErrorWithStatusCode{
		Error: relaymodel.Error{
			Message:  err.Error(),
			Type:     relaymodel.ErrorTypeGemini,
			Code:     "count_tokens_failed",
			RawError: err,
		},
		StatusCode: statusCode,
	}
}

// geminiCountTokensAPIError converts a Gemini error payload into a normalized API error.
// Parameters: statusCode is the HTTP status, geminiErr is the parsed Gemini error payload, and rawBody is the raw response body.
// Returns: a Gemini-shaped API error wrapper.
func geminiCountTokensAPIError(statusCode int, geminiErr *Error, rawBody []byte) *relaymodel.ErrorWithStatusCode {
	message := strings.TrimSpace(string(rawBody))
	code := any("count_tokens_failed")
	rawErr := errors.New(message)
	if geminiErr != nil {
		if trimmed := strings.TrimSpace(geminiErr.Message); trimmed != "" {
			message = trimmed
			rawErr = errors.New(trimmed)
		}
		if geminiErr.Code != 0 {
			code = geminiErr.Code
		}
	}

	return &relaymodel.ErrorWithStatusCode{
		Error: relaymodel.Error{
			Message:  message,
			Type:     relaymodel.ErrorTypeGemini,
			Code:     code,
			RawError: rawErr,
		},
		StatusCode: statusCode,
	}
}

// fallbackTextEmbeddingUsage estimates text-only embedding usage locally when countTokens is unavailable.
// Parameters: contents is the normalized text-only Gemini content list and modelName is the target embedding model.
// Returns: a best-effort prompt usage snapshot using the local text tokenizer.
func fallbackTextEmbeddingUsage(contents []ChatContent, modelName string) *relaymodel.Usage {
	promptTokens := 0
	for _, content := range contents {
		for _, part := range content.Parts {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			promptTokens += openai.CountTokenText(part.Text, modelName)
		}
	}

	return &relaymodel.Usage{
		PromptTokens: promptTokens,
		TotalTokens:  promptTokens,
	}
}

// buildPromptUsageFromCountTokens converts Gemini countTokens output into the relay usage shape.
// Parameters: response is the parsed Gemini countTokens response body.
// Returns: a prompt-only usage snapshot with modality details when present.
func buildPromptUsageFromCountTokens(response *CountTokensResponse) *relaymodel.Usage {
	if response == nil {
		return &relaymodel.Usage{}
	}
	usage := &relaymodel.Usage{
		PromptTokens: response.TotalTokens,
		TotalTokens:  response.TotalTokens,
	}
	if len(response.PromptTokensDetails) == 0 {
		return usage
	}

	details := &relaymodel.UsagePromptTokensDetails{}
	for _, detail := range response.PromptTokensDetails {
		switch strings.ToUpper(strings.TrimSpace(detail.Modality)) {
		case "", "MODALITY_UNSPECIFIED", "TEXT":
			details.TextTokens += detail.TokenCount
		case "IMAGE":
			details.ImageTokens += detail.TokenCount
		case "AUDIO":
			details.AudioTokens += detail.TokenCount
		case "VIDEO":
			details.VideoTokens += detail.TokenCount
		case "DOCUMENT":
			details.DocumentTokens += detail.TokenCount
		}
	}

	if hasAnyPromptTokenDetails(details) {
		usage.PromptTokensDetails = details
	}
	return usage
}

// hasAnyPromptTokenDetails reports whether any tracked embedding modality detail is populated.
// Parameters: details is the relay usage prompt-token detail snapshot.
// Returns: true when at least one tracked field is non-zero.
func hasAnyPromptTokenDetails(details *relaymodel.UsagePromptTokensDetails) bool {
	if details == nil {
		return false
	}
	return details.CachedTokens > 0 || details.AudioTokens > 0 || details.TextTokens > 0 ||
		details.ImageTokens > 0 || details.VideoTokens > 0 || details.DocumentTokens > 0 ||
		details.ImageCount > 0 || details.AudioSeconds > 0 || details.VideoFrames > 0 || details.DocumentPages > 0
}

// hasMultimodalPromptTokenDetails reports whether the prompt-token detail snapshot includes any non-text modality.
// Parameters: details is the relay usage prompt-token detail snapshot.
// Returns: true when at least one non-text embedding modality is populated.
func hasMultimodalPromptTokenDetails(details *relaymodel.UsagePromptTokensDetails) bool {
	if details == nil {
		return false
	}
	return details.ImageTokens > 0 || details.AudioTokens > 0 || details.VideoTokens > 0 ||
		details.DocumentTokens > 0 || details.ImageCount > 0 || details.AudioSeconds > 0 ||
		details.VideoFrames > 0 || details.DocumentPages > 0
}

// buildEmbeddingContentFromItem converts one normalized embedding input item into a Gemini content entry.
// Parameters: item is a single top-level embedding input entry.
// Returns: the content entry, whether it includes any non-text modality, and an error when the shape is unsupported.
func buildEmbeddingContentFromItem(item any) (ChatContent, bool, error) {
	switch value := item.(type) {
	case string:
		return ChatContent{Parts: []Part{{Text: value}}}, false, nil
	case map[string]any:
		return buildEmbeddingContentFromMap(value)
	default:
		return ChatContent{}, false, errors.Errorf("unsupported embedding content item type %T", item)
	}
}

// buildEmbeddingContentFromMap converts a content object or part object into a Gemini content entry.
// Parameters: raw is a map representing either a full content object or a single part object.
// Returns: the content entry, whether it includes any non-text modality, and an error when required fields are missing.
func buildEmbeddingContentFromMap(raw map[string]any) (ChatContent, bool, error) {
	if partsValue, ok := getMapValueAliases(raw, "parts"); ok {
		partsRaw, ok := partsValue.([]any)
		if !ok {
			return ChatContent{}, false, errors.New("embedding content parts must be an array")
		}
		if len(partsRaw) == 0 {
			return ChatContent{}, false, errors.New("embedding content parts must not be empty")
		}

		parts := make([]Part, 0, len(partsRaw))
		hasMultimodal := false
		for _, rawPart := range partsRaw {
			partMap, ok := rawPart.(map[string]any)
			if !ok {
				return ChatContent{}, false, errors.Errorf("embedding part must be an object, got %T", rawPart)
			}
			part, multimodal, err := buildEmbeddingPartFromMap(partMap)
			if err != nil {
				return ChatContent{}, false, errors.Wrap(err, "build embedding part")
			}
			if multimodal {
				hasMultimodal = true
			}
			parts = append(parts, part)
		}

		content := ChatContent{Parts: parts}
		if roleValue, ok := getMapValueAliases(raw, "role"); ok {
			if role, ok := roleValue.(string); ok {
				content.Role = role
			}
		}
		return content, hasMultimodal, nil
	}

	part, multimodal, err := buildEmbeddingPartFromMap(raw)
	if err != nil {
		return ChatContent{}, false, errors.Wrap(err, "build embedding part from map")
	}
	return ChatContent{Parts: []Part{part}}, multimodal, nil
}

// buildEmbeddingPartFromMap converts a single Gemini part object for embeddings.
// Parameters: raw is a map that may contain text, inline data, or file data fields.
// Returns: the normalized part, whether it represents a non-text modality, and an error when no supported fields are present.
func buildEmbeddingPartFromMap(raw map[string]any) (Part, bool, error) {
	if textValue, ok := getMapValueAliases(raw, "text"); ok {
		text, ok := textValue.(string)
		if !ok {
			return Part{}, false, errors.New("embedding text part must be a string")
		}
		return Part{Text: text}, false, nil
	}

	if inlineValue, ok := getMapValueAliases(raw, "inlineData", "inline_data"); ok {
		inlineMap, ok := inlineValue.(map[string]any)
		if !ok {
			return Part{}, false, errors.New("embedding inline_data part must be an object")
		}

		mimeTypeValue, hasMimeType := getMapValueAliases(inlineMap, "mimeType", "mime_type")
		dataValue, hasData := getMapValueAliases(inlineMap, "data")
		if !hasMimeType || !hasData {
			return Part{}, false, errors.New("embedding inline_data part requires mime_type and data")
		}

		mimeType, ok := mimeTypeValue.(string)
		if !ok {
			return Part{}, false, errors.New("embedding inline_data mime_type must be a string")
		}
		data, ok := dataValue.(string)
		if !ok {
			return Part{}, false, errors.New("embedding inline_data data must be a string")
		}

		return Part{
			InlineData: &InlineData{
				MimeType: mimeType,
				Data:     data,
			},
		}, true, nil
	}

	if fileValue, ok := getMapValueAliases(raw, "fileData", "file_data"); ok {
		fileMap, ok := fileValue.(map[string]any)
		if !ok {
			return Part{}, false, errors.New("embedding file_data part must be an object")
		}

		mimeTypeValue, _ := getMapValueAliases(fileMap, "mimeType", "mime_type")
		fileURIValue, hasFileURI := getMapValueAliases(fileMap, "fileUri", "file_uri")
		if !hasFileURI {
			return Part{}, false, errors.New("embedding file_data part requires file_uri")
		}

		fileURI, ok := fileURIValue.(string)
		if !ok {
			return Part{}, false, errors.New("embedding file_data file_uri must be a string")
		}

		fileData := &FileData{FileURI: fileURI}
		if mimeType, ok := mimeTypeValue.(string); ok {
			fileData.MimeType = mimeType
		}

		return Part{FileData: fileData}, true, nil
	}

	return Part{}, false, errors.New("embedding part must contain text, inline_data, or file_data")
}

// getMapValueAliases returns the first present value among a list of candidate keys.
// Parameters: raw is the source object and keys is the ordered list of accepted aliases.
// Returns: the found value and true when any alias exists.
func getMapValueAliases(raw map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			return value, true
		}
	}
	return nil, false
}
