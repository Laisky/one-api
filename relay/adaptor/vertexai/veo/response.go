package veo

import (
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// convert2OpenaiResponse writes a completed Veo operation to the client as an
// OpenAI image-style response whose data entries carry each video's URI or,
// when the provider inlined it, its base64 bytes.
// It accepts the gin context and the completed operation, and returns nil on
// success or the relay error to report.
func convert2OpenaiResponse(c *gin.Context, veoResp *PollVideoTaskResponse) *model.ErrorWithStatusCode {
	if veoResp == nil {
		return openai.ErrorWrapper(errors.New("VEO response is nil"), "veo_response_nil", http.StatusInternalServerError)
	}

	result := veoResp.Response
	imageDatas := make([]openai.ImageData, 0, len(result.Videos)+len(result.GeneratedSamples))
	// Vertex AI reports response.videos with gcsUri or bytesBase64Encoded.
	for _, video := range result.Videos {
		imageDatas = append(imageDatas, openai.ImageData{
			Url:     video.GcsUri,
			B64Json: video.BytesBase64Encoded,
		})
	}
	// Gemini Developer API style generatedSamples remain accepted.
	for _, sample := range result.GeneratedSamples {
		imageDatas = append(imageDatas, openai.ImageData{
			Url: sample.Video.URI,
		})
	}
	if len(imageDatas) == 0 {
		gmw.GetLogger(c).Warn("veo operation finished without videos",
			zap.String("operation", veoResp.Name),
			zap.Int("rai_media_filtered_count", result.RaiMediaFilteredCount))
	}

	openaiResp := &openai.ImageResponse{
		Created: helper.GetTimestamp(),
		Data:    imageDatas,
		// Usage for video generation is not directly provided by VEO in a token-based format.
		// The openai.ImageResponse.Usage field (type openai.ImageUsage) will be its zero value.
	}

	jsonResponse, err := json.Marshal(openaiResp)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "marshal veo response"), "marshal_openai_response_failed", http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	if _, err := c.Writer.Write(jsonResponse); err != nil {
		// If WriteHeader has been called, an error here is harder to report to the client cleanly.
		return openai.ErrorWrapper(errors.Wrap(err, "write veo response"), "write_response_body_failed", http.StatusInternalServerError)
	}

	return nil
}
