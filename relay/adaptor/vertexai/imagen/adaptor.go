package imagen

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// imagenDefaultImageConfig returns baseline image pricing metadata for Imagen models.
func imagenDefaultImageConfig(pricePerImage float64) *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: pricePerImage,
		DefaultSize:      "1024x1024",
		DefaultQuality:   "standard",
		MinImages:        1,
		SizeMultipliers: map[string]float64{
			"1024x1024": 1,
		},
	}
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on VertexAI Imagen pricing: https://cloud.google.com/vertex-ai/generative-ai/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// -------------------------------------
	// Imagen Pricing (source: official Vertex AI pricing doc, 2025-08)

	// Imagen 4.0 GA (2025-08-14)
	"imagen-4.0-generate-001":       {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.04)}, // Imagen 4
	"imagen-4.0-ultra-generate-001": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.06)}, // Imagen 4 Ultra
	"imagen-4.0-fast-generate-001":  {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)}, // Imagen 4 Fast

	// Imagen 4.0 Public Preview (retained for backward compatibility)
	"imagen-4.0-generate-preview-06-06":       {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.04)},
	"imagen-4.0-ultra-generate-preview-06-06": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.06)},
	"imagen-4.0-fast-generate-preview-06-06":  {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)},
	"imagen-4.0-generate-preview-05-20":       {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.04)},
	"imagen-4.0-ultra-generate-preview-05-20": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.06)},
	"imagen-4.0-fast-generate-preview-05-20":  {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)},

	// Imagen 3.0 (GA)
	"imagen-3.0-generate-001":      {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.04)}, // Imagen 3
	"imagen-3.0-generate-002":      {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.04)}, // Imagen 3
	"imagen-3.0-fast-generate-001": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)}, // Imagen 3 Fast
	"imagen-3.0-capability-001":    {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.04)}, // Imagen 3 edit/customize

	// Imagen 2.x & 1.x (legacy imagegeneration@ versions)
	"imagegeneration@006": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)}, // Imagen 2 (gen + edit)
	"imagegeneration@005": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)}, // Imagen 2 early GA
	"imagegeneration@002": {Ratio: 0, CompletionRatio: 1.0, Image: imagenDefaultImageConfig(0.02)}, // Imagen 1 (gen + edit)
}

// ModelList derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {
	// No initialization needed
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	meta := meta.GetByContext(c)

	if request.ResponseFormat == nil || *request.ResponseFormat != "b64_json" {
		return nil, errors.New("only support b64_json response format")
	}
	if request.N <= 0 {
		request.N = 1 // Default to 1 if not specified
	}

	switch meta.Mode {
	case relaymode.ImagesGenerations:
		return convertImageCreateRequest(request)
	case relaymode.ImagesEdits:
		switch c.ContentType() {
		// case "application/json":
		// 	return ConvertJsonImageEditRequest(c)
		case "multipart/form-data":
			return ConvertMultipartImageEditRequest(c)
		default:
			return nil, errors.New("unsupported content type for image edit")
		}
	default:
		return nil, errors.New("not implemented")
	}
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, wrapErr *model.ErrorWithStatusCode) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewBuffer(respBody))

	switch meta.Mode {
	case relaymode.ImagesEdits:
		return HandleImageEdit(c, resp)
	case relaymode.ImagesGenerations:
		return nil, handleImageGeneration(c, resp, respBody)
	default:
		return nil, openai.ErrorWrapper(errors.New("unsupported mode"), "unsupported_mode", http.StatusBadRequest)
	}
}

func handleImageGeneration(c *gin.Context, resp *http.Response, respBody []byte) *model.ErrorWithStatusCode {
	var imageResponse CreateImageResponse

	if resp.StatusCode != http.StatusOK {
		return openai.ErrorWrapper(errors.New(string(respBody)), "imagen_api_error", resp.StatusCode)
	}

	err := json.Unmarshal(respBody, &imageResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	// Convert to OpenAI format
	openaiResp := openai.ImageResponse{
		Created: time.Now().Unix(),
		Data:    make([]openai.ImageData, 0, len(imageResponse.Predictions)),
	}

	for _, prediction := range imageResponse.Predictions {
		openaiResp.Data = append(openaiResp.Data, openai.ImageData{
			B64Json: prediction.BytesBase64Encoded,
		})
	}

	respBytes, err := json.Marshal(openaiResp)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(respBytes)
	if err != nil {
		return openai.ErrorWrapper(err, "write_response_failed", http.StatusInternalServerError)
	}

	return nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "vertex_ai_imagen"
}

func convertImageCreateRequest(request *model.ImageRequest) (any, error) {
	return CreateImageRequest{
		Instances: []createImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: createImageParameters{
			SampleCount: request.N,
		},
	}, nil
}
