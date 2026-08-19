package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"mime/multipart"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

type TextContent struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type ImageContent struct {
	Type     string          `json:"type,omitempty"`
	ImageURL *model.ImageURL `json:"image_url,omitempty"`
}

type ChatRequest struct {
	Model     string          `json:"model"`
	Messages  []model.Message `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type TextRequest struct {
	Model     string          `json:"model"`
	Messages  []model.Message `json:"messages"`
	Prompt    string          `json:"prompt"`
	MaxTokens int             `json:"max_tokens"`
	//Stream   bool      `json:"stream"`
}

// ImageRequest docs: https://platform.openai.com/docs/api-reference/images/create
type ImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt" binding:"required"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"`
	User           string `json:"user,omitempty"`
}

type WhisperJSONResponse struct {
	Text string `json:"text,omitempty"`
}

type WhisperVerboseJSONResponse struct {
	Task     string    `json:"task,omitempty"`
	Language string    `json:"language,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	Text     string    `json:"text,omitempty"`
	Segments []Segment `json:"segments,omitempty"`
}

type Segment struct {
	Id               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float64 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
}

type TextToSpeechRequest struct {
	Model          string  `json:"model" binding:"required"`
	Input          string  `json:"input" binding:"required"`
	Voice          string  `json:"voice" binding:"required"`
	Speed          float64 `json:"speed"`
	ResponseFormat string  `json:"response_format"`
}

type AudioTranscriptionRequest struct {
	File                 *multipart.FileHeader `form:"file" binding:"required"`
	Model                string                `form:"model" binding:"required"`
	Language             string                `form:"language"`
	Prompt               string                `form:"prompt"`
	ReponseFormat        string                `form:"response_format" binding:"oneof=json text srt verbose_json vtt"`
	Temperature          float64               `form:"temperature"`
	TimestampGranularity []string              `form:"timestamp_granularity"`
}

type AudioTranslationRequest struct {
	File           *multipart.FileHeader `form:"file" binding:"required"`
	Model          string                `form:"model" binding:"required"`
	Prompt         string                `form:"prompt"`
	ResponseFormat string                `form:"response_format" binding:"oneof=json text srt verbose_json vtt"`
	Temperature    float64               `form:"temperature"`
}

type UsageOrResponseText struct {
	*model.Usage
	ResponseText string
}

type SlimTextResponse struct {
	Choices     []TextResponseChoice `json:"choices"`
	model.Usage `json:"usage"`
	Error       *model.Error `json:"error,omitempty"`
}

type TextResponseChoice struct {
	Index         int `json:"index"`
	model.Message `json:"message"`
	FinishReason  string `json:"finish_reason"`
}

type TextResponse struct {
	Id          string               `json:"id"`
	Model       string               `json:"model,omitempty"`
	Object      string               `json:"object"`
	Created     int64                `json:"created"`
	Choices     []TextResponseChoice `json:"choices"`
	model.Usage `json:"usage"`
}

type EmbeddingResponseItem struct {
	Object        string    `json:"object"`
	Index         int       `json:"index"`
	Embedding     []float64 `json:"embedding"`
	Base64Encoded bool      `json:"-"`
}

// UnmarshalJSON supports embedding vectors delivered either as numeric arrays or
// base64-encoded float32 blobs (Azure/OpenAI encoding_format=base64).
func (item *EmbeddingResponseItem) UnmarshalJSON(data []byte) error {
	type rawEmbeddingResponseItem struct {
		Object    string          `json:"object"`
		Index     int             `json:"index"`
		Embedding json.RawMessage `json:"embedding"`
	}
	var raw rawEmbeddingResponseItem
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal embedding response item")
	}
	item.Object = raw.Object
	item.Index = raw.Index
	item.Base64Encoded = false
	if len(raw.Embedding) == 0 {
		item.Embedding = nil
		return nil
	}
	trimmed := bytes.TrimSpace(raw.Embedding)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		item.Embedding = nil
		return nil
	}
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &item.Embedding); err != nil {
			return errors.Wrap(err, "decode numeric embedding")
		}
	case '"':
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err != nil {
			return errors.Wrap(err, "decode base64 embedding string")
		}
		floats, err := decodeBase64Embedding(encoded)
		if err != nil {
			return errors.Wrap(err, "convert base64 embedding payload")
		}
		item.Embedding = floats
		item.Base64Encoded = true
	default:
		preview := trimmed
		if len(preview) > 32 {
			preview = preview[:32]
		}
		return errors.Errorf("unsupported embedding encoding prefix %q", string(preview))
	}
	return nil
}

type EmbeddingResponse struct {
	Object      string                  `json:"object"`
	Data        []EmbeddingResponseItem `json:"data"`
	Model       string                  `json:"model"`
	model.Usage `json:"usage"`
	Error       *model.Error `json:"error,omitempty"`
}

// ImageData represents an image in the response
type ImageData struct {
	Url           string `json:"url,omitempty"`
	B64Json       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageResponse represents the response structure for image generations
type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
	Usage   ImageUsage  `json:"usage"`
}

type ChatCompletionsStreamResponseChoice struct {
	Index        int           `json:"index"`
	Delta        model.Message `json:"delta"`
	FinishReason *string       `json:"finish_reason,omitempty"`
}

// ChatCompletionsStreamResponse is the streaming response structure for chat completions
type ChatCompletionsStreamResponse struct {
	Id      string                                `json:"id"`
	Object  string                                `json:"object"`
	Created int64                                 `json:"created"`
	Model   string                                `json:"model"`
	Choices []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage   *model.Usage                          `json:"usage,omitempty"`
}

// CompletionsStreamResponse represents the response structure
// for text completions in streaming mode
type CompletionsStreamResponse struct {
	Choices []struct {
		Text         string `json:"text"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ImageUsage is the usage info for image request
//
// https://platform.openai.com/docs/api-reference/images/object
//
// TODO: To ensure compatibility with other providers that use the OpenAI format,
// we may need to explicitly add 'omitempty' and potentially 'omitzero' JSON tags,
// as the current implementation always returns zero values in JSON.
type ImageUsage struct {
	TotalTokens        int                          `json:"total_tokens"`
	InputTokens        int                          `json:"input_tokens"`
	OutputTokens       int                          `json:"output_tokens"`
	InputTokensDetails ImageUsageInputTokensDetails `json:"input_tokens_details"`
}

// ImageUsageInputTokensDetails is the details of input tokens for image request
type ImageUsageInputTokensDetails struct {
	TextTokens  int `json:"text_tokens"`
	ImageTokens int `json:"image_tokens"`
}

// Convert2GeneralUsage converts ImageUsage to model.Usage
func (u *ImageUsage) Convert2GeneralUsage() *model.Usage {
	return &model.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
		PromptTokensDetails: &model.UsagePromptTokensDetails{
			ImageTokens: u.InputTokensDetails.ImageTokens,
			TextTokens:  u.InputTokensDetails.TextTokens,
		},
	}
}

func decodeBase64Embedding(encoded string) ([]float64, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "decode base64 string")
	}
	if len(raw)%4 != 0 {
		return nil, errors.Errorf("invalid base64 embedding byte length %d", len(raw))
	}
	values := make([]float64, len(raw)/4)
	for i := 0; i < len(values); i++ {
		bits := binary.LittleEndian.Uint32(raw[i*4 : (i+1)*4])
		values[i] = float64(math.Float32frombits(bits))
	}
	return values, nil
}
