package jina

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// normalizeSearchResponse preserves opaque vectors/results while normalizing usage.
// Jina's total_tokens includes all billable input, including images and repeated
// query/document processing. Missing usage is an error, not an invented zero bill.
func normalizeSearchResponse(body []byte, mode int) ([]byte, *model.Usage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, errors.Wrap(err, "decode jina search response")
	}
	resultKey := "data"
	if mode == relaymode.Rerank {
		resultKey = "results"
	}
	if raw := bytes.TrimSpace(payload[resultKey]); len(raw) == 0 || raw[0] != '[' {
		return nil, nil, errors.Errorf("jina response has no %s array", resultKey)
	}
	var upstream struct {
		TotalTokens *int `json:"total_tokens"`
	}
	if err := json.Unmarshal(payload["usage"], &upstream); err != nil {
		return nil, nil, errors.Wrap(err, "decode jina usage")
	}
	if upstream.TotalTokens == nil || *upstream.TotalTokens < 0 {
		return nil, nil, errors.New("jina response has missing or negative total_tokens")
	}
	usage := &model.Usage{PromptTokens: *upstream.TotalTokens, TotalTokens: *upstream.TotalTokens}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload["usage"], &fields); err != nil {
		return nil, nil, errors.Wrap(err, "decode jina usage fields")
	}
	fields["prompt_tokens"] = json.RawMessage(strconv.Itoa(usage.PromptTokens))
	fields["completion_tokens"] = json.RawMessage("0")
	usageJSON, err := json.Marshal(fields)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encode jina usage")
	}
	payload["usage"] = usageJSON
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encode jina search response")
	}
	return encoded, usage, nil
}

// handleSearchResponse closes the upstream body and emits normalized JSON once.
func handleSearchResponse(c *gin.Context, resp *http.Response, mode int) (*model.Usage, *model.ErrorWithStatusCode) {
	if resp == nil || resp.Body == nil {
		return nil, openai.ErrorWrapper(errors.New("jina response body is nil"), "invalid_upstream_response", http.StatusBadGateway)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusBadGateway)
	}
	body, usage, err := normalizeSearchResponse(body, mode)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "invalid_upstream_response", http.StatusBadGateway)
	}
	c.Data(resp.StatusCode, "application/json", body)
	return usage, nil
}
