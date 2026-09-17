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

// normalizeSearchResponse preserves opaque vectors/results while normalizing
// usage. Valid receipt evidence survives a malformed results/data envelope.
// Duplicate keys, missing usage and invalid counters never become a zero bill.
func normalizeSearchResponse(body []byte, mode int) ([]byte, *model.Usage, error) {
	payload, err := uniqueObject(body)
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode jina search response")
	}
	fields, err := uniqueObject(payload["usage"])
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode jina search usage")
	}
	total, err := tokenCount(fields, "total_tokens")
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode jina search total tokens")
	}
	usage := &model.Usage{PromptTokens: total, TotalTokens: total}
	resultKey := "data"
	if mode == relaymode.Rerank {
		resultKey = "results"
	}
	if raw := bytes.TrimSpace(payload[resultKey]); len(raw) == 0 || raw[0] != '[' {
		return nil, usage, errors.Errorf("jina response has no %s array", resultKey)
	}
	fields["prompt_tokens"] = json.RawMessage(strconv.Itoa(total))
	fields["completion_tokens"] = json.RawMessage("0")
	usageJSON, err := json.Marshal(fields)
	if err != nil {
		return nil, usage, errors.Wrap(err, "encode jina usage")
	}
	payload["usage"] = usageJSON
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, usage, errors.Wrap(err, "encode jina search response")
	}
	return encoded, usage, nil
}

// handleSearchResponse bounds reads, preserves known billed usage on errors and
// emits normalized JSON once. Unknown/zero usage for an admitted nonempty input
// retains a labelled conservative allowance instead of making work free.
func handleSearchResponse(c *gin.Context, resp *http.Response, mode int) (usage *model.Usage, apiErr *model.ErrorWithStatusCode) {
	if resp == nil || resp.Body == nil {
		return EstimatedUsage(c, "jina_missing_search_body"), openai.ErrorWrapper(errors.New("jina response body is nil"), "invalid_upstream_response", http.StatusBadGateway)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil && apiErr == nil {
			apiErr = openai.ErrorWrapper(errors.Wrap(err, "close jina search body"), "invalid_upstream_response", http.StatusBadGateway)
		}
	}()
	const bodyLimit = 64 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		if err == nil {
			err = errors.New("jina response exceeds 64 MiB limit")
		}
		return EstimatedUsage(c, "jina_unreadable_search_body"), openai.ErrorWrapper(err, "read_response_body_failed", http.StatusBadGateway)
	}
	encoded, usage, err := normalizeSearchResponse(body, mode)
	if usage == nil || usage.TotalTokens == 0 {
		if estimate := EstimatedUsage(c, "jina_missing_invalid_or_zero_search_usage"); estimate != nil {
			usage = estimate
			if evidence := partialReceiptEvidence(body); evidence != nil {
				usage.PromptTokens = max(usage.PromptTokens, evidence.TotalTokens, evidence.PromptTokens)
				usage.TotalTokens = usage.PromptTokens
			}
			if err == nil {
				err = errors.New("jina returned zero usage for nonempty admitted input")
			}
		}
	}
	if err != nil {
		return usage, openai.ErrorWrapper(err, "invalid_upstream_response", http.StatusBadGateway)
	}
	c.Data(resp.StatusCode, "application/json", encoded)
	return usage, nil
}
