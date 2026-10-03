package cohere

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

const embedContractKey = "cohere_embed_contract"

// embedContract records the validated output shape without changing the input DTO.
type embedContract struct {
	Count      int
	Dimensions int
	Encoding   string
}

// embedRequest is the v2 Embed text contract, not the unrelated chat payload.
type embedRequest struct {
	Model           string   `json:"model"`
	Texts           []string `json:"texts"`
	InputType       string   `json:"input_type"`
	EmbeddingTypes  []string `json:"embedding_types"`
	OutputDimension int      `json:"output_dimension,omitempty"`
	Truncate        string   `json:"truncate,omitempty"`
	Priority        *int     `json:"priority,omitempty"`
}

// convertEmbedRequest maps standard text embedding input to Cohere v2. Input-type
// and truncation extensions are recovered from the body, while mapped routing
// fields remain authoritative. Unsupported token-ID or multimodal input fails
// before any provider request rather than silently embedding its JSON string.
func convertEmbedRequest(c *gin.Context, request *model.GeneralOpenAIRequest) (*embedRequest, error) {
	if request.Stream {
		return nil, errors.New("Cohere embeddings do not support streaming")
	}
	texts := []string{}
	switch input := request.Input.(type) {
	case string:
		texts = append(texts, input)
	case []string:
		texts = append(texts, input...)
	case []any:
		for _, item := range input {
			text, ok := item.(string)
			if !ok {
				return nil, errors.New("Cohere standard embeddings require text strings, not token IDs or media")
			}
			texts = append(texts, text)
		}
	default:
		return nil, errors.New("Cohere standard embeddings require a string or string array")
	}
	if len(texts) == 0 || len(texts) > 96 {
		return nil, errors.New("Cohere embeddings require between 1 and 96 texts")
	}
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("Cohere embedding text must not be empty")
		}
	}
	encoding := request.EncodingFormat
	if encoding == "" {
		encoding = "float"
	}
	if encoding != "float" && encoding != "base64" {
		return nil, errors.New("encoding_format must be float or base64")
	}
	dimensions := request.Dimensions
	if dimensions != 0 && (request.Model != "embed-v4.0" || !slices.Contains([]int{256, 512, 1024, 1536}, dimensions)) {
		return nil, errors.New("dimensions is supported only for embed-v4.0: 256, 512, 1024, or 1536")
	}
	out := &embedRequest{Model: request.Model, Texts: texts, InputType: "search_document", EmbeddingTypes: []string{"float"}, OutputDimension: dimensions}
	raw := map[string]json.RawMessage{}
	if request.ExtraBody != nil {
		for _, key := range []string{"input_type", "truncate", "priority"} {
			if value, ok := request.ExtraBody[key]; ok {
				encoded, err := json.Marshal(value)
				if err != nil {
					return nil, errors.Wrap(err, "encode embedding option")
				}
				raw[key] = encoded
			}
		}
	}
	if c != nil && c.Request != nil {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return nil, errors.Wrap(err, "read embedding options")
		}
		if len(body) > 0 {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				return nil, errors.Wrap(err, "decode embedding options")
			}
			for _, key := range []string{"input_type", "truncate", "priority"} {
				if value, ok := fields[key]; ok {
					raw[key] = value
				}
			}
		}
	}
	if value, ok := raw["input_type"]; ok {
		if err := json.Unmarshal(value, &out.InputType); err != nil {
			return nil, errors.Wrap(err, "decode input_type")
		}
	}
	if !slices.Contains([]string{"search_document", "search_query", "classification", "clustering"}, out.InputType) {
		return nil, errors.New("input_type is not a text embedding task")
	}
	if value, ok := raw["truncate"]; ok {
		if err := json.Unmarshal(value, &out.Truncate); err != nil {
			return nil, errors.Wrap(err, "decode truncate")
		}
	}
	if out.Truncate != "" && !slices.Contains([]string{"NONE", "START", "END"}, out.Truncate) {
		return nil, errors.New("truncate must be NONE, START, or END")
	}
	if value, ok := raw["priority"]; ok {
		if err := json.Unmarshal(value, &out.Priority); err != nil {
			return nil, errors.Wrap(err, "decode priority")
		}
	}
	if out.Priority != nil && (*out.Priority < 0 || *out.Priority > 999) {
		return nil, errors.New("priority must be between 0 and 999")
	}
	if c != nil {
		c.Set(embedContractKey, embedContract{len(texts), dimensions, encoding})
	}
	return out, nil
}

// embedResponse normalizes ordered embeddings and bills the provider's billed
// units, not its larger raw token counts. A valid receipt survives client write
// failures. Missing receipts are surfaced as an explicitly labelled estimate.
func embedResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_embedding_response", http.StatusBadGateway)
	}
	if len(body) > 32<<20 {
		return nil, openai.ErrorWrapper(errors.New("embedding response too large"), "invalid_embedding_response", http.StatusBadGateway)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, buildRerankError(body, resp.StatusCode)
	}
	var payload struct {
		Embeddings struct {
			Float [][]float64 `json:"float"`
		} `json:"embeddings"`
		Meta struct {
			BilledUnits *struct {
				InputTokens int `json:"input_tokens"`
				ImageTokens int `json:"image_tokens"`
			} `json:"billed_units"`
		} `json:"meta"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, openai.ErrorWrapper(err, "invalid_embedding_response", http.StatusBadGateway)
	}
	if payload.Message != "" {
		return nil, buildRerankError(body, http.StatusBadGateway)
	}
	usage := &model.Usage{}
	units := payload.Meta.BilledUnits
	if units == nil {
		usage.PromptTokens = m.PromptTokens
		usage.BillingEstimateReason = "cohere_embedding_billed_units_missing"
	} else {
		if units.InputTokens < 0 || units.ImageTokens < 0 || units.InputTokens > math.MaxInt-units.ImageTokens {
			return nil, openai.ErrorWrapper(errors.New("invalid billed token counts"), "invalid_embedding_usage", http.StatusBadGateway)
		}
		usage.PromptTokens = units.InputTokens + units.ImageTokens
		usage.PromptTokensDetails = &model.UsagePromptTokensDetails{TextTokens: units.InputTokens, ImageTokens: units.ImageTokens}
	}
	usage.TotalTokens = usage.PromptTokens
	contract := embedContract{}
	if raw, ok := c.Get(embedContractKey); ok {
		contract, _ = raw.(embedContract)
	}
	if len(payload.Embeddings.Float) == 0 || (contract.Count > 0 && len(payload.Embeddings.Float) != contract.Count) {
		return usage, openai.ErrorWrapper(errors.New("upstream embedding count differs from input count"), "invalid_embedding_response", http.StatusBadGateway)
	}
	data := make([]map[string]any, len(payload.Embeddings.Float))
	dimension := contract.Dimensions
	if dimension == 0 {
		dimension = len(payload.Embeddings.Float[0])
	}
	for index, vector := range payload.Embeddings.Float {
		if dimension == 0 || len(vector) != dimension {
			return usage, openai.ErrorWrapper(errors.New("upstream embedding dimensions differ"), "invalid_embedding_response", http.StatusBadGateway)
		}
		var embedding any = vector
		if contract.Encoding == "base64" {
			encoded := make([]byte, 4*len(vector))
			for i, value := range vector {
				if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > math.MaxFloat32 {
					return usage, openai.ErrorWrapper(errors.New("embedding is outside float32 range"), "invalid_embedding_response", http.StatusBadGateway)
				}
				binary.LittleEndian.PutUint32(encoded[i*4:], math.Float32bits(float32(value)))
			}
			embedding = base64.StdEncoding.EncodeToString(encoded)
		}
		data[index] = map[string]any{"object": "embedding", "index": index, "embedding": embedding}
	}
	visible := m.OriginModelName
	if visible == "" {
		visible = m.ActualModelName
	}
	encoded, err := json.Marshal(map[string]any{"object": "list", "model": visible, "data": data, "usage": usage})
	if err != nil {
		return usage, openai.ErrorWrapper(err, "encode_embedding_response", http.StatusBadGateway)
	}
	c.Header("Content-Type", "application/json")
	if id := resp.Header.Get("X-Request-ID"); id != "" {
		c.Header("X-Request-ID", id)
	}
	c.Status(http.StatusOK)
	if _, err := c.Writer.Write(encoded); err != nil {
		return usage, openai.ErrorWrapper(err, "write_embedding_response", http.StatusInternalServerError)
	}
	return usage, nil
}
