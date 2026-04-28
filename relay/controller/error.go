package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

type GeneralErrorResponse struct {
	Error    model.Error `json:"error"`
	Message  string      `json:"message"`
	Msg      string      `json:"msg"`
	Err      string      `json:"err"`
	ErrorMsg string      `json:"error_msg"`
	Header   struct {
		Message string `json:"message"`
	} `json:"header"`
	Response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
}

func (e GeneralErrorResponse) ToMessage() string {
	if e.Error.Message != "" {
		return e.Error.Message
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != "" {
		return e.Err
	}
	if e.ErrorMsg != "" {
		return e.ErrorMsg
	}
	if e.Header.Message != "" {
		return e.Header.Message
	}
	if e.Response.Error.Message != "" {
		return e.Response.Error.Message
	}
	return ""
}

// RelayErrorHandler parses upstream error responses into our unified error model.
// For request-scoped logging, prefer RelayErrorHandlerWithContext.
func RelayErrorHandler(resp *http.Response) (ErrorWithStatusCode *model.ErrorWithStatusCode) {
	if resp == nil {
		return &model.ErrorWithStatusCode{
			StatusCode: 500,
			Error: model.Error{
				Message:  "resp is nil",
				Type:     model.ErrorTypeUpstream,
				Code:     "bad_response",
				RawError: errors.New("resp is nil"),
			},
		}
	}

	ErrorWithStatusCode = &model.ErrorWithStatusCode{
		StatusCode: resp.StatusCode,
		Error: model.Error{
			Message:  "",
			Type:     model.ErrorTypeUpstream,
			Code:     "bad_response_status_code",
			Param:    strconv.Itoa(resp.StatusCode),
			RawError: errors.Errorf("bad response %d", resp.StatusCode),
		},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("failed to read response body: %+v\n\n", err) + ErrorWithStatusCode.Error.Message
		ErrorWithStatusCode.Error.RawError = err
		return
	}

	// Intentionally avoid global logger here; use context variant where possible.
	err = resp.Body.Close()
	if err != nil {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("failed to close response body: %+v\n\n", err) + ErrorWithStatusCode.Error.Message
		ErrorWithStatusCode.Error.RawError = err
		return
	}

	var errResponse GeneralErrorResponse
	err = json.Unmarshal(responseBody, &errResponse)
	if err != nil {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("failed to unmarshal response body: %+v\n\n", err) + ErrorWithStatusCode.Error.Message + string(responseBody)
		ErrorWithStatusCode.Error.RawError = err
		return
	}

	if errResponse.Error.Message != "" {
		// OpenAI format error, so we override the default one
		ErrorWithStatusCode.Error = errResponse.Error
		if ErrorWithStatusCode.Error.RawError == nil && ErrorWithStatusCode.Error.Message != "" {
			ErrorWithStatusCode.Error.RawError = errors.New(ErrorWithStatusCode.Error.Message)
		}
	} else {
		ErrorWithStatusCode.Error.Message = errResponse.ToMessage()
		if ErrorWithStatusCode.Error.RawError == nil && ErrorWithStatusCode.Error.Message != "" {
			ErrorWithStatusCode.Error.RawError = errors.New(ErrorWithStatusCode.Error.Message)
		}
	}

	if ErrorWithStatusCode.Error.Message == "" {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("bad response [%d] %s", resp.StatusCode, string(responseBody))
	}

	return
}

// RelayErrorHandlerWithContext is a context-aware variant that logs using the request-scoped logger.
func RelayErrorHandlerWithContext(c *gin.Context, resp *http.Response) *model.ErrorWithStatusCode {
	if resp == nil {
		return RelayErrorHandler(resp)
	}
	// Read and restore response body for downstream use
	responseBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	logUpstreamResponseFromBytes(gmw.GetLogger(c), resp, responseBody, string(model.ErrorTypeUpstream))
	// Reconstruct a new ReadCloser for any further reads (not commonly needed here)
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	return RelayErrorHandler(resp)
}
