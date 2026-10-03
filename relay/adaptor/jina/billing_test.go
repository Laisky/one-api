package jina

import (
	"encoding/json"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestBillingBudgetSearchCountsAllWork verifies image admission never estimates
// a URL as text tokens, and top_n/dimensions do not reduce submitted work.
func TestBillingBudgetSearchCountsAllWork(t *testing.T) {
	t.Parallel()
	request := &model.GeneralOpenAIRequest{Model: "jina-embeddings-v4", Input: []any{"hello", map[string]any{"image": "https://example.com/image.png"}}}
	budget, err := QuoteRequest(request, relaymode.Embeddings)
	require.NoError(t, err)
	require.GreaterOrEqual(t, budget.Input, 2*32768)
	request.Dimensions = 1
	request.EncodingFormat = "base64"
	other, err := QuoteRequest(request, relaymode.Embeddings)
	require.NoError(t, err)
	require.Equal(t, budget, other)
	one := 1
	rerank := &model.RerankRequest{Model: "jina-reranker-v3.5", Query: "query", Documents: []string{"a", "b"}}
	all, err := QuoteRerank(rerank)
	require.NoError(t, err)
	rerank.TopN = &one
	limited, err := QuoteRerank(rerank)
	require.NoError(t, err)
	require.Equal(t, all, limited)
	rerank.Documents = append(rerank.Documents, "c")
	larger, err := QuoteRerank(rerank)
	require.NoError(t, err)
	require.Greater(t, larger.Input, all.Input)
}

// TestBillingBudgetRejectsUnknownLiability checks unsupported modalities, batch
// bounds, unverified models and wrong endpoint/model combinations before dispatch.
func TestBillingBudgetRejectsUnknownLiability(t *testing.T) {
	t.Parallel()
	for _, input := range []any{nil, []any{}, map[string]any{"pdf": "https://example.com/a.pdf"}, map[string]any{"audio": "https://example.com/a.mp3"}, map[string]any{"video": "https://example.com/a.mp4"}, map[string]any{"content": []any{"group"}}, make([]any, 513), strings.Repeat("x", 1024*1024+1)} {
		_, err := QuoteRequest(&model.GeneralOpenAIRequest{Model: "jina-embeddings-v5-omni-small", Input: input}, relaymode.Embeddings)
		require.Error(t, err)
	}
	_, err := QuoteRequest(&model.GeneralOpenAIRequest{Model: "unknown", Input: "text"}, relaymode.Embeddings)
	require.Error(t, err)
	_, err = QuoteRequest(&model.GeneralOpenAIRequest{Model: "jina-reranker-v3.5", Input: "text"}, relaymode.Embeddings)
	require.Error(t, err)
	for _, name := range []string{"jina-reranker-v3.5", "jina-reranker-m0"} {
		r := &model.RerankRequest{Model: name, Query: "q", Documents: []string{"[image]"}, NativeDocuments: []json.RawMessage{json.RawMessage(`{"image":"https://example.com/a.png"}`)}}
		_, err := QuoteRerank(r)
		if name == "jina-reranker-m0" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

// TestBillingBudgetOCRLimits verifies a real upstream output cap is always set
// and cannot be bypassed with unsupported tools, n, or a conflicting legacy cap.
func TestBillingBudgetOCRLimits(t *testing.T) {
	t.Parallel()
	req := &model.GeneralOpenAIRequest{Model: "jina-ocr-v1", Messages: []model.Message{{Role: "user", Content: "read image"}}}
	budget, err := QuoteRequest(req, relaymode.ChatCompletions)
	require.NoError(t, err)
	require.Equal(t, 8192, budget.Output)
	wire, err := (&Adaptor{}).ConvertRequest(nil, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.Equal(t, 8192, *wire.(*model.GeneralOpenAIRequest).MaxCompletionTokens)
	for _, limit := range []int{-1, 0, 8193} {
		req.MaxCompletionTokens = &limit
		_, err := QuoteRequest(req, relaymode.ChatCompletions)
		require.Error(t, err)
	}
	limit := 32
	req.MaxCompletionTokens = &limit
	req.MaxTokens = 8193
	budget, err = QuoteRequest(req, relaymode.ChatCompletions)
	require.NoError(t, err)
	require.Equal(t, 32, budget.Output)
	n := 2
	req.N = &n
	_, err = QuoteRequest(req, relaymode.ChatCompletions)
	require.Error(t, err)
}

// TestBillingBudgetPriceArithmetic pins units, rounding, free groups, configured
// rate overrides and non-finite/overflow rejection without a database dependency.
func TestBillingBudgetPriceArithmetic(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		budget               BillingBudget
		input, output, group float64
		want                 int64
	}{
		{BillingBudget{Input: 1}, .025, 0, 1, 1},
		{BillingBudget{Input: 40}, .025, 0, 1, 1},
		{BillingBudget{Input: 41}, .025, 0, 1, 2},
		{BillingBudget{Input: 1000000}, .025, 0, 1, 25000},
		{BillingBudget{Input: 1000000}, .01, 0, 1, 10000},
		{BillingBudget{Input: 100, Output: 20}, .25, 4, 1, 45},
		{BillingBudget{Input: 100, Output: 20}, .25, 4, 2, 90},
		{BillingBudget{Input: 100, Output: 20}, .25, 4, 0, 0},
	} {
		got, err := BudgetQuota(tc.budget, tc.input, tc.output, tc.group)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		_, err := BudgetQuota(BillingBudget{Input: 100, Output: 1}, rate, 4, 1)
		require.Error(t, err)
	}
	minimum, err := BudgetQuota(BillingBudget{Input: 1}, math.SmallestNonzeroFloat64, 0, math.SmallestNonzeroFloat64)
	require.NoError(t, err)
	require.EqualValues(t, 1, minimum, "a positive decimal fee cannot underflow into a free request")
	_, err = BudgetQuota(BillingBudget{Input: MaxBillingTokens + 1}, .025, 0, 1)
	require.Error(t, err)
}

// TestOCRReceiptObserver verifies fragment boundaries, final usage, cumulative
// duplicate snapshots, missing receipts and ambiguous counters independently of
// the shared stream handler's token-estimation fallback.
func TestOCRReceiptObserver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, wire     string
		stream, valid  bool
		prompt, output int
	}{
		{"json", `{"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`, false, true, 100, 20},
		{"sse", "data: {\"usage\":null}\n\ndata: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\ndata: [DONE]\n", true, true, 100, 20},
		{"repeat", "data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\ndata: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n", true, true, 100, 20},
		{"missing", "data: {\"choices\":[{\"delta\":{\"content\":\"text\"}}]}\n", true, false, 0, 0},
		{"truncated", `{"usage":{"prompt_tokens":100`, false, false, 0, 0},
		{"duplicate", `{"usage":{"prompt_tokens":100,"prompt_tokens":1,"completion_tokens":20,"total_tokens":120}}`, false, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observer := &receiptBody{ReadCloser: io.NopCloser(strings.NewReader(tc.wire)), stream: tc.stream}
			buffer := make([]byte, 7)
			for {
				_, err := observer.Read(buffer)
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			require.NoError(t, observer.Close())
			valid := observer.receipt != nil && !observer.invalid
			require.Equal(t, tc.valid, valid)
			if tc.valid {
				require.Equal(t, tc.prompt, observer.receipt.PromptTokens)
				require.Equal(t, tc.output, observer.receipt.CompletionTokens)
			}
		})
	}
}

// FuzzBillingBudgetQuota checks nonnegative, representable admission and
// monotonicity for bounded integer inputs without making paid upstream requests.
func FuzzBillingBudgetQuota(f *testing.F) {
	f.Add(uint32(40), uint32(2), uint16(25))
	f.Add(uint32(1000000), uint32(1000), uint16(1))
	f.Fuzz(func(t *testing.T, input, output uint32, rate uint16) {
		budget := BillingBudget{Input: int(input % 1000000), Output: int(output % 8193)}
		ratio := float64(rate) / 1000
		before, err := BudgetQuota(budget, ratio, 4, 1)
		require.NoError(t, err)
		require.GreaterOrEqual(t, before, int64(0))
		budget.Input++
		after, err := BudgetQuota(budget, ratio, 4, 1)
		require.NoError(t, err)
		require.GreaterOrEqual(t, after, before)
	})
}
