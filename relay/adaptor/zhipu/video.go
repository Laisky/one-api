package zhipu

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// VideoHandler passes through the native Zhipu videos/generations response
// (an asynchronous task envelope), detects Zhipu's error payloads, and binds
// the returned task id so follow-up status/content requests pin to this channel.
//
// Parameters: c is the gin context, resp is the upstream HTTP response.
// Returns: a business error when Zhipu reports failure, nil otherwise, along
// with nil usage (video generation is billed per call, not per token).
func VideoHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if err = resp.Body.Close(); err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Zhipu wraps failures as {"error":{"code":...,"message":...}}.
	var maybeError struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if len(body) > 0 {
		if unmarshalErr := json.Unmarshal(body, &maybeError); unmarshalErr == nil {
			if maybeError.Error != nil && maybeError.Error.Message != "" {
				return &model.ErrorWithStatusCode{
					Error: model.Error{
						Message:  maybeError.Error.Message,
						Type:     model.ErrorTypeZhipu,
						Code:     maybeError.Error.Code,
						RawError: errors.New(maybeError.Error.Message),
					},
					StatusCode: resp.StatusCode,
				}, nil
			}
		}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  string(body),
				Type:     model.ErrorTypeZhipu,
				RawError: errors.New(string(body)),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}

	if c.Request.Method == http.MethodPost {
		openai.PersistAsyncVideoTask(c, body)
	}

	for k, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(body); err != nil {
		return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, nil
}
