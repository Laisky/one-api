package imagen

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// ConvertImageEditRequest handles the conversion from multipart form to Imagen request format
func ConvertMultipartImageEditRequest(c *gin.Context) (*CreateImageRequest, error) {
	// Recover request body for binding
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

	// Parse the form
	var rawReq model.OpenaiImageEditRequest
	if err := c.ShouldBind(&rawReq); err != nil {
		return nil, errors.Wrap(err, "parse image edit form")
	}

	// Validate response format
	if rawReq.ResponseFormat != "b64_json" {
		return nil, errors.New("response_format must be b64_json for Imagen models")
	}

	// Set default N if not provided
	if rawReq.N <= 0 {
		rawReq.N = 1
	}

	// Set default edit mode if not provided
	editMode := "EDIT_MODE_INPAINT_INSERTION"
	if rawReq.EditMode != nil {
		editMode = *rawReq.EditMode
	}

	// Set default mask mode if not provided
	maskMode := "MASK_MODE_USER_PROVIDED"
	if rawReq.MaskMode != nil {
		maskMode = *rawReq.MaskMode
	}

	// Process the image file
	imgFile, err := rawReq.Image.Open()
	if err != nil {
		return nil, errors.Wrap(err, "open image file")
	}
	defer imgFile.Close()

	imgData, err := io.ReadAll(imgFile)
	if err != nil {
		return nil, errors.Wrap(err, "read image file")
	}

	// Process the mask file
	maskFile, err := rawReq.Mask.Open()
	if err != nil {
		return nil, errors.Wrap(err, "open mask file")
	}
	defer maskFile.Close()

	maskData, err := io.ReadAll(maskFile)
	if err != nil {
		return nil, errors.Wrap(err, "read mask file")
	}

	// Convert to base64
	imgBase64 := base64.StdEncoding.EncodeToString(imgData)
	maskBase64 := base64.StdEncoding.EncodeToString(maskData)

	// Create the request
	req := &CreateImageRequest{
		Instances: []createImageInstance{
			{
				Prompt: rawReq.Prompt,
				ReferenceImages: []ReferenceImage{
					{
						ReferenceType: "REFERENCE_TYPE_RAW",
						ReferenceId:   1,
						ReferenceImage: ReferenceImageData{
							BytesBase64Encoded: imgBase64,
						},
					},
					{
						ReferenceType: "REFERENCE_TYPE_MASK",
						ReferenceId:   2,
						ReferenceImage: ReferenceImageData{
							BytesBase64Encoded: maskBase64,
						},
						MaskImageConfig: &MaskImageConfig{
							MaskMode: maskMode,
						},
					},
				},
			},
		},
		Parameters: createImageParameters{
			SampleCount: rawReq.N,
			EditMode:    &editMode,
		},
	}

	return req, nil
}

// HandleImageEdit processes an image edit response from Imagen API
func HandleImageEdit(c *gin.Context, resp *http.Response) (*model.Usage, *model.ErrorWithStatusCode) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, openai.ErrorWrapper(errors.New(string(respBody)), "imagen_api_error", resp.StatusCode)
	}

	var imageResponse CreateImageResponse
	err = json.Unmarshal(respBody, &imageResponse)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
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
		return nil, openai.ErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(respBytes)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "write_response_failed", http.StatusInternalServerError)
	}

	// Create usage data (minimal as this API doesn't provide token counts)
	usage := &model.Usage{
		PromptTokens:     0,
		CompletionTokens: 0,
		TotalTokens:      0,
	}

	return usage, nil
}
