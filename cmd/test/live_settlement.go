package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	livert "github.com/Laisky/one-api/relay/realtime"
)

// verifyLiveSettlement matches the session against its persisted consume log.
// Parameters: ctx, logger and opts describe the run, requestID identifies the
// upgraded request and receipts are the observed provider receipts. Returns:
// an error when settlement is missing, incomplete or disagrees with the receipts.
func verifyLiveSettlement(ctx context.Context, logger glog.Logger, opts liveOptions,
	requestID string, receipts []map[string]any) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("request ID is required to verify Live settlement")
	}
	wantPrompt, wantCompletion, err := expectedLiveTokens(receipts)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, liveSettlementTimeout)
	defer cancel()
	var last string
	for {
		entries, err := fetchLiveSettlementEntries(ctx, opts, requestID)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.RequestID != requestID {
				continue
			}
			last = entry.Content
			// Only an untouched reservation may still become a final settlement.
			if !entry.Metadata.BillingComplete {
				if entry.Metadata.Usage.Receipts == 0 && !entry.Metadata.Usage.UsageGap {
					continue
				}
				return errors.Errorf("settled log is incomplete with %d receipts and usage_gap=%t: %s",
					entry.Metadata.Usage.Receipts, entry.Metadata.Usage.UsageGap, entry.Content)
			}
			if entry.Metadata.Usage.Receipts != len(receipts) {
				return errors.Errorf("settled log reports %d receipts, want %d: %s",
					entry.Metadata.Usage.Receipts, len(receipts), entry.Content)
			}
			if entry.Metadata.Usage.UsageGap {
				return errors.Errorf("settled log reports a usage gap: %s", entry.Content)
			}
			if int64(entry.PromptTokens) != wantPrompt || int64(entry.CompletionTokens) != wantCompletion {
				return errors.Errorf("settled tokens %d/%d do not match the provider receipts %d/%d",
					entry.PromptTokens, entry.CompletionTokens, wantPrompt, wantCompletion)
			}
			logger.Info("session settled",
				zap.Int64("quota", entry.Quota), zap.Int("prompt_tokens", entry.PromptTokens),
				zap.Int("completion_tokens", entry.CompletionTokens),
				zap.Int("receipts", entry.Metadata.Usage.Receipts), zap.String("content", entry.Content))
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "no settled consume log for request %q (last seen: %s)", requestID, last)
		case <-time.After(time.Second):
		}
	}
}

// expectedLiveTokens normalizes receipts using the production decoder instead
// of maintaining a second, drifting billing formula. Parameters: receipts are
// observed usageMetadata objects. Returns: token totals or a validation error.
// Independent hand-calculated vectors belong in the regression tests; this
// probe checks persistence consistency, not independent provider invoice prices.
func expectedLiveTokens(receipts []map[string]any) (prompt, completion int64, err error) {
	for _, usage := range receipts {
		tokens, err := decodeLiveReceipt(usage)
		if err != nil {
			return 0, 0, err
		}
		prompt += tokens.Input
		completion += tokens.Output
	}
	return prompt, completion, nil
}

// decodeLiveReceipt validates one observed receipt. Parameters: usage is the
// decoded usageMetadata. Returns: normalized tokens or the production error.
func decodeLiveReceipt(usage map[string]any) (livert.Tokens, error) {
	payload, err := json.Marshal(usage)
	if err != nil {
		return livert.Tokens{}, errors.Wrap(err, "encode Live receipt")
	}
	record, err := livert.DecodeGeminiUsage(payload)
	if err != nil {
		return livert.Tokens{}, errors.Wrap(err, "validate Live receipt")
	}
	return record.Tokens, nil
}

// assertLiveReceipt validates accounting evidence for a paid conversation.
// Parameters: usage is the decoded usageMetadata. Returns: an error for invalid
// counters, impossible partitions, or a receipt without the expected input.
func assertLiveReceipt(usage map[string]any) error {
	tokens, err := decodeLiveReceipt(usage)
	if err != nil {
		return err
	}
	if tokens.Input <= 0 {
		return errors.New("receipt has no promptTokenCount")
	}
	return nil
}

// fetchLiveSettlementEntries searches bounded pages for exactly one session.
// Parameters: ctx bounds all requests, opts supplies credentials and requestID
// identifies the row. Returns: the matching row, no rows yet, or an error. No
// model filter is sent: persisted provider model names may differ from aliases.
func fetchLiveSettlementEntries(ctx context.Context, opts liveOptions, requestID string) ([]consumeLogEntry, error) {
	const maxPages = 100
	for page := 0; page < maxPages; page++ {
		entries, total, err := fetchConsumeLogPage(ctx, opts.apiBase, opts.apiToken, "", page)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.RequestID == requestID {
				return []consumeLogEntry{entry}, nil
			}
		}
		if len(entries) == 0 || (page+1)*20 >= total {
			return nil, nil
		}
	}
	return nil, errors.Errorf("request %q was not found within the %d-page settlement search budget", requestID, maxPages)
}

// liveReceiptCount reads a numeric receipt field shared with the existing
// OpenAI Realtime probe. Parameters: usage is a decoded receipt and key names
// its counter. Returns: the reported value and whether the counter was present.
// Gemini Live uses decodeLiveReceipt for strict validation instead.
func liveReceiptCount(usage map[string]any, key string) (int64, bool) {
	value, exists := usage[key]
	if !exists {
		return 0, false
	}
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int64(number), true
}
