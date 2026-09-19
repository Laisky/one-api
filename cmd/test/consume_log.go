package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// consumeLogEntry is the subset of a consume-log row this probe asserts on.
type consumeLogEntry struct {
	RequestID        string `json:"request_id"`
	CreatedAt        int64  `json:"created_at"`
	Quota            int64  `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Content          string `json:"content"`
	Metadata         struct {
		BillingComplete bool `json:"realtime_billing_complete"`
		Usage           struct {
			Receipts int  `json:"receipt_count"`
			UsageGap bool `json:"usage_gap"`
		} `json:"realtime_usage"`
	} `json:"metadata"`
}

// fetchConsumeLogs reads this token's recent consume logs. Parameters: ctx
// bounds the call, apiBase and token address the server, and model optionally
// narrows rows by model. Returns: the rows, newest first, or an error.
func fetchConsumeLogs(ctx context.Context, apiBase, token, model string) ([]consumeLogEntry, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	endpoint := apiBase + "/api/token/logs?size=20"
	if strings.TrimSpace(model) != "" {
		endpoint += "&model_name=" + url.QueryEscape(model)
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build consume log request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "fetch consume logs")
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, errors.Wrap(err, "read consume logs")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("consume log request failed with %d: %s", resp.StatusCode, snippet(payload))
	}
	var body struct {
		Success bool              `json:"success"`
		Data    []consumeLogEntry `json:"data"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, errors.Wrap(err, "decode consume logs")
	}
	if !body.Success {
		return nil, errors.Errorf("consume log request was not successful: %s", snippet(payload))
	}
	return body.Data, nil
}
