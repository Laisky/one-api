package zhipu

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// ConvertVoiceCloneRequest returns the Zhipu-native voice-clone payload. Zhipu
// expects the canonical DTO shape (model/voice_name/text/input/file_id/
// request_id), so the request is forwarded as-is with the model already mapped
// by the controller.
//
// Parameters: c is the gin context and request is the validated voice-clone DTO.
// Returns: the request itself, or an error when the request is nil.
func (a *Adaptor) ConvertVoiceCloneRequest(_ *gin.Context, request *model.VoiceCloneRequest) (any, error) {
	if request == nil {
		return nil, errors.New("voice clone request is nil")
	}
	return request, nil
}

// DoVoiceCloneResponse passes through the native Zhipu voice-clone response
// (voice id, preview file id, request id), detecting Zhipu's error envelope.
//
// Parameters: c is the gin context, resp is the upstream HTTP response, and
// meta is unused (kept for interface parity).
// Returns: a business error when Zhipu reports failure, nil otherwise, along
// with nil usage (voice cloning is billed per call, not per token).
func (a *Adaptor) DoVoiceCloneResponse(c *gin.Context, resp *http.Response, _ *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	}

	var maybeError struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if len(body) > 0 {
		if unmarshalErr := json.Unmarshal(body, &maybeError); unmarshalErr == nil {
			if maybeError.Error != nil && maybeError.Error.Message != "" {
				return nil, &model.ErrorWithStatusCode{
					Error: model.Error{
						Message:  maybeError.Error.Message,
						Type:     model.ErrorTypeZhipu,
						Code:     maybeError.Error.Code,
						RawError: errors.New(maybeError.Error.Message),
					},
					StatusCode: resp.StatusCode,
				}
			}
		}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  string(body),
				Type:     model.ErrorTypeZhipu,
				RawError: errors.New(string(body)),
			},
			StatusCode: resp.StatusCode,
		}
	}

	for k, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(body); err != nil {
		return nil, openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError)
	}
	return nil, nil
}
