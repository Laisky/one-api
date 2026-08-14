package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// ImagesEditsHandler just copy response body to client
//
// https://platform.openai.com/docs/api-reference/images/createEdit
// func ImagesEditsHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
// 	c.Writer.WriteHeader(resp.StatusCode)
// 	for k, v := range resp.Header {
// 		c.Writer.Header().Set(k, v[0])
// 	}

// 	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
// 		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
// 	}
// 	defer resp.Body.Close()

// 	return nil, nil
// }

func ImageHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	var imageResponse ImageResponse
	responseBody, err := io.ReadAll(resp.Body)

	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		copyImageErrorHeaders(c.Writer.Header(), resp.Header)
		upstreamErr := buildImageUpstreamError(responseBody, resp.StatusCode)
		if lg != nil {
			fields := []zap.Field{
				zap.Int("status", resp.StatusCode),
				zap.Int("response_bytes", len(responseBody)),
				zap.String("error_type", string(upstreamErr.Type)),
				zap.String("error_code", formatImageErrorCode(upstreamErr.Code)),
			}
			if upstreamErr.Param != "" {
				fields = append(fields, zap.String("error_param", boundImageLogField(upstreamErr.Param)))
			}
			if requestID := resp.Header.Get("x-request-id"); requestID != "" {
				fields = append(fields, zap.String("upstream_request_id", boundImageLogField(requestID)))
			}
			lg.Debug("upstream image request returned a structured error", fields...)
		}
		return upstreamErr, nil
	}

	err = json.Unmarshal(responseBody, &imageResponse)
	if err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)

	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		// Return usage even on write failure so billing can proceed for forwarded requests
		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), imageResponse.Usage.Convert2GeneralUsage()
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, imageResponse.Usage.Convert2GeneralUsage()
}

// buildImageUpstreamError converts an upstream image error payload into the relay's
// structured error while preserving the upstream HTTP status and safe diagnostic fields.
// Parameters: responseBody is the upstream JSON body and statusCode is its HTTP status.
// Returns: an error suitable for the relay retry, logging, and client-response pipeline.
func buildImageUpstreamError(responseBody []byte, statusCode int) *model.ErrorWithStatusCode {
	var payload struct {
		Error   *model.Error    `json:"error"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Type    model.ErrorType `json:"type"`
		Param   string          `json:"param"`
		Code    any             `json:"code"`
	}
	if err := json.Unmarshal(responseBody, &payload); err == nil {
		var upstreamErr model.Error
		if payload.Error != nil {
			upstreamErr = *payload.Error
		} else {
			upstreamErr = model.Error{
				Message: payload.Message,
				Type:    payload.Type,
				Param:   payload.Param,
				Code:    payload.Code,
			}
			if upstreamErr.Message == "" {
				upstreamErr.Message = payload.Msg
			}
		}

		if upstreamErr.Message != "" || upstreamErr.Code != nil {
			if upstreamErr.Type == model.ErrorTypeUnknown {
				upstreamErr.Type = model.ErrorTypeUpstream
			}
			if upstreamErr.Code == nil {
				upstreamErr.Code = "upstream_http_error"
			}
			if upstreamErr.Message == "" {
				upstreamErr.Message = "upstream image request failed"
			}
			upstreamErr.Message = boundImageErrorMessage(upstreamErr.Message)
			upstreamErr.Param = boundImageLogField(upstreamErr.Param)
			upstreamErr.Code = boundImageErrorCode(upstreamErr.Code)
			upstreamErr.RawError = errors.New(upstreamErr.Message)
			return &model.ErrorWithStatusCode{Error: upstreamErr, StatusCode: statusCode}
		}
	}

	fallback := errors.Errorf("upstream image request failed with status %d", statusCode)
	return &model.ErrorWithStatusCode{
		Error: model.Error{
			Message:  fallback.Error(),
			Type:     model.ErrorTypeUpstream,
			Code:     "upstream_http_error",
			RawError: fallback,
		},
		StatusCode: statusCode,
	}
}

// formatImageErrorCode formats an arbitrary upstream error code for structured logs
// without serializing the entire upstream error payload.
// Parameters: code is the provider-supplied error code value.
// Returns: a compact string representation, or an empty string when no code exists.
func formatImageErrorCode(code any) string {
	if code == nil {
		return ""
	}
	if value, ok := code.(string); ok {
		return boundImageLogField(value)
	}
	encoded, err := json.Marshal(code)
	if err != nil {
		return ""
	}
	return boundImageLogField(string(encoded))
}

const (
	imageLogFieldMaxRunes     = 256
	imageErrorMessageMaxRunes = 2048
)

// boundImageLogField limits provider-controlled structured log values while
// preserving valid UTF-8. Parameters: value is an upstream field. Returns: the
// original value when short enough, or a rune-safe truncated representation.
func boundImageLogField(value string) string {
	runes := []rune(value)
	if len(runes) <= imageLogFieldMaxRunes {
		return value
	}
	return string(runes[:imageLogFieldMaxRunes])
}

// boundImageErrorMessage limits provider-controlled error messages before they
// reach client responses and downstream relay logs. Parameters: value is an
// upstream message. Returns: a rune-safe value capped at the diagnostic limit.
func boundImageErrorMessage(value string) string {
	runes := []rune(value)
	if len(runes) <= imageErrorMessageMaxRunes {
		return value
	}
	return string(runes[:imageErrorMessageMaxRunes])
}

// boundImageErrorCode normalizes provider-controlled error codes before later
// relay stages log them. Parameters: code may be a scalar or arbitrary decoded
// JSON. Returns: scalar codes unchanged except bounded strings; composite codes
// become bounded JSON strings.
func boundImageErrorCode(code any) any {
	switch value := code.(type) {
	case nil, bool, float64:
		return value
	case string:
		return boundImageLogField(value)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "upstream_http_error"
		}
		return boundImageLogField(string(encoded))
	}
}

// copyImageErrorHeaders copies only retry, rate-limit, and request-correlation
// metadata from an upstream image error. Parameters: dst is the client response
// header and src is the upstream header. Returns: none; credential, cookie,
// transport, and content headers are deliberately excluded.
func copyImageErrorHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		lowerKey := strings.ToLower(key)
		isSafe := lowerKey == "retry-after" ||
			lowerKey == "x-request-id" ||
			lowerKey == "request-id" ||
			lowerKey == "apim-request-id" ||
			lowerKey == "x-ms-request-id" ||
			strings.HasPrefix(lowerKey, "x-ratelimit-") ||
			strings.HasPrefix(lowerKey, "ratelimit-")
		if !isSafe {
			continue
		}
		for _, value := range values {
			dst.Add(key, boundImageLogField(value))
		}
	}
}
