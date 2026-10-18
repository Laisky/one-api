package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// NativeErrorResponse retains bounded, validated wire bytes and their exact
// normalized error identity. It never commits output during retry selection.
type NativeErrorResponse struct {
	body    []byte
	headers http.Header
	status  int
	failure *model.ErrorWithStatusCode
}

type nativeErrorReplayBody struct {
	io.Reader
	io.Closer
}

// prepareNativeErrorResponse reads at most limit+1 bytes and restores the exact
// prefix plus remaining body for the existing generic error parser. Parameters:
// resp is upstream and limit bounds retained bytes. Returns: a complete native
// error candidate, or nil on oversize, read failure or a non-native envelope.
func prepareNativeErrorResponse(resp *http.Response, limit int64) *NativeErrorResponse {
	if resp == nil || resp.Body == nil || resp.StatusCode < 400 || resp.StatusCode > 599 || limit < 0 {
		return nil
	}
	original := resp.Body
	body, err := io.ReadAll(io.LimitReader(original, limit+1))
	resp.Body = &nativeErrorReplayBody{Reader: io.MultiReader(bytes.NewReader(body), original), Closer: original}
	if err != nil || int64(len(body)) > limit {
		return nil
	}
	var envelope struct {
		Type  string `json:"type"`
		Error *struct {
			Type    string  `json:"type"`
			Message *string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Type != "error" || envelope.Error == nil || envelope.Error.Type == "" || envelope.Error.Message == nil {
		return nil
	}
	headers := make(http.Header)
	CopyClaudeResponseHeaders(headers, resp.Header)
	return &NativeErrorResponse{body: body, headers: headers, status: resp.StatusCode}
}

// PrepareNativeErrorResponse retains a native error within the same 32 MiB
// ceiling as native JSON success. Parameters: resp is the current upstream HTTP
// error. Returns: a validated candidate without writing any downstream output.
func PrepareNativeErrorResponse(resp *http.Response) *NativeErrorResponse {
	return prepareNativeErrorResponse(resp, maxClaudeHTTPPayload)
}

// RetainNativeErrorResponse binds a candidate to the normalized failure returned
// by this exact attempt. Parameters: c is request-scoped, candidate is validated
// wire data, and failure is the returned relay error. Returns: nothing.
func RetainNativeErrorResponse(c *gin.Context, candidate *NativeErrorResponse, failure *model.ErrorWithStatusCode) {
	if c == nil || candidate == nil || failure == nil || failure.StatusCode != candidate.status {
		return
	}
	candidate.failure = failure
	c.Set(ctxkey.ClaudeNativeErrorResponse, candidate)
}

// WriteNativeErrorResponse commits a captured native error only after the outer
// retry loop selects its matching failure. Parameters: c is the downstream
// context and failure is final. Returns: whether matching native bytes were sent.
func WriteNativeErrorResponse(c *gin.Context, failure *model.ErrorWithStatusCode) bool {
	if c == nil {
		return false
	}
	value, _ := c.Get(ctxkey.ClaudeNativeErrorResponse)
	c.Set(ctxkey.ClaudeNativeErrorResponse, nil)
	candidate, ok := value.(*NativeErrorResponse)
	if !ok || candidate == nil || failure == nil || candidate.failure != failure || candidate.status != failure.StatusCode || c.Writer == nil || c.Writer.Written() {
		return false
	}
	CopyClaudeResponseHeaders(c.Writer.Header(), candidate.headers)
	c.Data(candidate.status, "application/json", candidate.body)
	return true
}
