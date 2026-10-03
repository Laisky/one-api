package ali

import (
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type Input struct {
	//Prompt   string       `json:"prompt"`
	Messages []Message `json:"messages"`
}

type Parameters struct {
	TopP              *float64     `json:"top_p,omitempty"`
	TopK              *int         `json:"top_k,omitempty"`
	Seed              uint64       `json:"seed,omitempty"`
	EnableSearch      bool         `json:"enable_search,omitempty"`
	IncrementalOutput bool         `json:"incremental_output,omitempty"`
	MaxTokens         int          `json:"max_tokens,omitempty"`
	Temperature       *float64     `json:"temperature,omitempty"`
	ResultFormat      string       `json:"result_format,omitempty"`
	Tools             []model.Tool `json:"tools,omitempty"`
}

type ChatRequest struct {
	Model      string     `json:"model"`
	Input      Input      `json:"input"`
	Parameters Parameters `json:"parameters"`
}

type ImageRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt         string `json:"prompt"`
		NegativePrompt string `json:"negative_prompt,omitempty"`
	} `json:"input"`
	Parameters struct {
		Size  string `json:"size,omitempty"`
		N     int    `json:"n,omitempty"`
		Steps string `json:"steps,omitempty"`
		Scale string `json:"scale,omitempty"`
	} `json:"parameters"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type TaskResponse struct {
	StatusCode int    `json:"status_code,omitempty"`
	RequestId  string `json:"request_id,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Output     struct {
		TaskId     string `json:"task_id,omitempty"`
		TaskStatus string `json:"task_status,omitempty"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		Results    []struct {
			B64Image string `json:"b64_image,omitempty"`
			Url      string `json:"url,omitempty"`
			Code     string `json:"code,omitempty"`
			Message  string `json:"message,omitempty"`
		} `json:"results,omitempty"`
		TaskMetrics struct {
			Total     int `json:"TOTAL,omitempty"`
			Succeeded int `json:"SUCCEEDED,omitempty"`
			Failed    int `json:"FAILED,omitempty"`
		} `json:"task_metrics"`
	} `json:"output"`
	Usage Usage `json:"usage"`
}

type Header struct {
	Action       string `json:"action,omitempty"`
	Streaming    string `json:"streaming,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	Event        string `json:"event,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Attributes   any    `json:"attributes,omitempty"`
}

type Payload struct {
	Model      string `json:"model,omitempty"`
	Task       string `json:"task,omitempty"`
	TaskGroup  string `json:"task_group,omitempty"`
	Function   string `json:"function,omitempty"`
	Parameters struct {
		SampleRate int     `json:"sample_rate,omitempty"`
		Rate       float64 `json:"rate,omitempty"`
		Format     string  `json:"format,omitempty"`
	} `json:"parameters"`
	Input struct {
		Text string `json:"text,omitempty"`
	} `json:"input"`
	Usage struct {
		Characters int `json:"characters,omitempty"`
	} `json:"usage"`
}

type WSSMessage struct {
	Header  Header  `json:"header"`
	Payload Payload `json:"payload"`
}

type EmbeddingRequest struct {
	Model string `json:"model"`
	Input struct {
		Texts []string `json:"texts"`
	} `json:"input"`
	Parameters *struct {
		TextType string `json:"text_type,omitempty"`
	} `json:"parameters,omitempty"`
}

type Embedding struct {
	Embedding []float64 `json:"embedding"`
	TextIndex int       `json:"text_index"`
}

type EmbeddingResponse struct {
	Output struct {
		Embeddings []Embedding `json:"embeddings"`
	} `json:"output"`
	Usage Usage `json:"usage"`
	Error
}

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"request_id"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
	// PromptTokensDetails carries the implicit/explicit context-cache hit count for
	// text models on the DashScope-native protocol. Alibaba reports the cached
	// portion of the input under usage.prompt_tokens_details.cached_tokens, and that
	// portion is billed at a discounted rate (20% of the standard input price).
	// See https://www.alibabacloud.com/help/en/model-studio/context-cache
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	// CachedTokens is the top-level cache-hit count reported by the multimodal
	// (e.g. qwen-vl) DashScope-native shape, which places it directly under usage
	// alongside input_tokens_details rather than nested in prompt_tokens_details.
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// PromptTokensDetails mirrors the DashScope usage.prompt_tokens_details object.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type Output struct {
	//Text         string                      `json:"text"`
	//FinishReason string                      `json:"finish_reason"`
	Choices []openai.TextResponseChoice `json:"choices"`
}

type ChatResponse struct {
	Output Output `json:"output"`
	Usage  Usage  `json:"usage"`
	Error
}
