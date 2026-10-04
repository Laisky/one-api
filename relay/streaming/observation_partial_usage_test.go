package streaming

import (
	relaymodel "github.com/Laisky/one-api/relay/model"
	"strings"
	"testing"
)

// TestObserveMessagesPartialReceiptCountsSameFrame preserves observed content
// while keeping complete measured receipts, including zero, authoritative.
func TestObserveMessagesPartialReceiptCountsSameFrame(t *testing.T) {
	reason := "reasoning"
	tests := []struct {
		name      string
		messages  []relaymodel.Message
		usage     *relaymodel.Usage
		want      int
		wantCount bool
	}{
		{"partial_text", []relaymodel.Message{{Content: "same-frame output"}}, &relaymodel.Usage{PromptTokens: 3, BillingEstimateReason: "stream_usage_missing_counters"}, len("same-frame output"), true},
		{"partial_reasoning_aliases", []relaymodel.Message{{Reasoning: &reason, ReasoningContent: &reason, Thinking: &reason}}, &relaymodel.Usage{PromptTokens: 3, BillingEstimateReason: "stream_usage_missing_counters"}, len(reason), true},
		{"partial_multiple_choices", []relaymodel.Message{{Content: "first"}, {Content: "second"}}, &relaymodel.Usage{PromptTokens: 3, BillingEstimateReason: "stream_usage_missing_counters"}, len("firstsecond"), true},
		{"partial_cumulative_max", []relaymodel.Message{{Content: "short"}}, &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 40, BillingEstimateReason: "stream_usage_missing_counters"}, 40, true},
		{"measured_receipt", []relaymodel.Message{{Content: strings.Repeat("visible ", 10)}}, &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}, 2, false},
		{"measured_zero", []relaymodel.Message{{Content: "visible"}}, &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 0, TotalTokens: 3}, 0, false},
		{"missing_receipt", []relaymodel.Message{{Content: "unmeasured"}}, nil, len("unmeasured"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			tracker := NewQuotaTracker(QuotaTrackerParams{ModelName: "gpt-4", PromptTokens: 3, ModelRatio: 1, GroupRatio: 1, PreConsumedQuota: 1000000, TokenCounter: func(s, _ string) int { calls++; return len(s) }})
			before := relaymodel.Usage{}
			if tc.usage != nil {
				before = *tc.usage
			}
			if err := tracker.ObserveMessages(tc.messages, tc.usage); err != nil {
				t.Fatal(err)
			}
			got := tracker.UsageSnapshot()
			if got.CompletionTokens != tc.want {
				t.Fatalf("completion=%d want=%d tokenizer calls=%d", got.CompletionTokens, tc.want, calls)
			}
			if (calls > 0) != tc.wantCount {
				t.Fatalf("tokenizer calls=%d should count=%v", calls, tc.wantCount)
			}
			if tc.usage != nil && (tc.usage.PromptTokens != before.PromptTokens || tc.usage.CompletionTokens != before.CompletionTokens || tc.usage.BillingEstimateReason != before.BillingEstimateReason) {
				t.Fatal("caller-owned receipt mutated")
			}
		})
	}
}
