package streamusage

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// Observer keeps billable SDK evidence independently of successful SSE delivery.
// The request goroutine is its sole owner; it performs no background drain.
type Observer struct {
	c                  *gin.Context
	usage              *relaymodel.Usage
	tracker            *streaming.QuotaTracker
	model              string
	prompt, completion int
	receipt, stopped   bool
	err                error
	// localErr records that err was produced by one-api itself while rendering
	// to the client (quota accounting, chunk encoding or a client write), not by
	// the provider's stream.
	localErr bool
}

// New initializes an explicitly estimated prompt rather than an authoritative
// non-nil zero receipt. The controller retains the separate admission quote.
func New(c *gin.Context, usage *relaymodel.Usage) *Observer {
	o := &Observer{c: c, usage: usage, tracker: streaming.FromContext(c), model: c.GetString(ctxkey.RequestModel)}
	if meta := metalib.GetByContext(c); meta != nil {
		o.prompt = meta.PromptTokens
	}
	if o.tracker != nil {
		streaming.ClaimProtocolObservation(c)
	}
	o.snapshot()
	return o
}

// snapshot updates only an unmeasured result, leaving a valid SDK receipt intact.
func (o *Observer) snapshot() {
	if !o.receipt {
		o.usage.PromptTokens = o.prompt
		o.usage.CompletionTokens = o.completion
		o.usage.TotalTokens = o.prompt + o.completion
		o.usage.BillingEstimateReason = "aws_stream_usage_missing_or_partial"
	}
}

// Fail remembers the first meaningful stop condition without erasing usage.
// Callers pass provider stream errors (stream.Err()) and cancellation here;
// failureFromUpstream decides the attribution when the stream finishes.
//
// Parameters:
//   - err: the stop condition; nil is ignored.
func (o *Observer) Fail(err error) {
	if err != nil && o.err == nil {
		o.err = err
	}
}

// failLocal remembers err like Fail and, when it is the first stop condition,
// marks it as produced by one-api rather than by the provider. Render uses it
// for quota accounting, chunk encoding and client write failures.
//
// Parameters:
//   - err: the local failure; nil is ignored.
func (o *Observer) failLocal(err error) {
	if err != nil && o.err == nil {
		o.err = err
		o.localErr = true
	}
}

// failureFromUpstream reports whether the recorded stop condition was caused by
// the provider's stream: an SDK stream error, invalid usage counters or a stream
// that ended without its stop and usage receipt. Local render failures, quota
// exhaustion, cancellation, deadlines and client disconnects keep one-api
// attribution, because a channel must not be penalized for them.
//
// Parameters:
//   - disconnected: true when the client went away during the stream.
//
// Returns:
//   - bool: true only for provider-caused failures.
func (o *Observer) failureFromUpstream(disconnected bool) bool {
	return o.err != nil &&
		!disconnected &&
		!o.localErr &&
		!errors.Is(o.err, streaming.ErrQuotaExceeded) &&
		!errors.Is(o.err, context.Canceled) &&
		!errors.Is(o.err, context.DeadlineExceeded)
}

// Render accounts for text, reasoning and tool arguments before delivery. Paid
// observations beyond available funds stop the stream but remain settleable.
func (o *Observer) Render(c *gin.Context, chunk *openai.ChatCompletionsStreamResponse) error {
	if o.err != nil {
		return o.err
	}
	delta := 0
	for _, choice := range chunk.Choices {
		delta += openai.CountTokenText(choice.Delta.StringContent(), o.model)
		seen := map[string]bool{}
		for _, part := range []*string{choice.Delta.ReasoningContent, choice.Delta.Reasoning, choice.Delta.Thinking} {
			if part != nil && !seen[*part] {
				seen[*part] = true
				delta += openai.CountTokenText(*part, o.model)
			}
		}
		for _, tool := range choice.Delta.ToolCalls {
			if tool.Function != nil {
				var text string
				switch value := tool.Function.Arguments.(type) {
				case string:
					text = value
				case nil:
				default:
					raw, err := json.Marshal(value)
					if err != nil {
						o.failLocal(err)
						return err
					}
					text = string(raw)
				}
				delta += openai.CountTokenText(text, o.model)
			}
		}
	}
	if delta > 0 {
		o.completion += delta
		if o.receipt {
			o.receipt = false
		}
		o.snapshot()
		if o.tracker != nil {
			if err := o.tracker.RecordCompletionTokens(delta); err != nil {
				o.failLocal(err)
				return err
			}
		}
	}
	err := openai_compatible.RenderStreamChunkWithBridge(c, chunk)
	o.failLocal(err)
	return err
}

// RecordMetadata accepts only complete nonnegative counters as measured usage;
// missing/invalid metadata cannot reset already observed work to zero.
func (o *Observer) RecordMetadata(value *types.TokenUsage) bool {
	if value == nil || value.InputTokens == nil || value.OutputTokens == nil || *value.InputTokens < 0 || *value.OutputTokens < 0 {
		o.Fail(errors.New("missing or invalid AWS stream usage counters"))
		return false
	}
	prompt, output := int(*value.InputTokens), int(*value.OutputTokens)
	if value.TotalTokens != nil && int(*value.TotalTokens) != prompt+output {
		o.Fail(errors.New("inconsistent AWS stream usage counters"))
		return false
	}
	o.prompt = prompt
	o.completion = output
	o.receipt = true
	*o.usage = relaymodel.Usage{PromptTokens: prompt, CompletionTokens: output, TotalTokens: prompt + output}
	if o.tracker != nil {
		o.tracker.UpdateFinalUsage(o.usage)
	}
	return true
}

// RecordStop records protocol completion independently of usage completeness.
func (o *Observer) RecordStop() { o.stopped = true }

// Complete is required before emitting a successful terminal SSE sequence.
func (o *Observer) Complete() bool { return o.receipt && o.stopped && o.err == nil }

// Finish prefers authoritative receipts, otherwise returns an explicitly marked
// estimate together with the failure; downstream cancellation never makes it free.
// The client-visible error keeps its one_api_error envelope and status, while
// FromUpstream records whether the provider caused the failure (see
// failureFromUpstream): a provider stream error is subject to the provider
// failure policy, a local quota or render failure is not.
//
// Parameters:
//   - disconnected: true when the client went away during the stream.
//   - streamErr: the SDK stream error (stream.Err()), or nil.
//
// Returns:
//   - *relaymodel.ErrorWithStatusCode: the failure, or nil for a complete stream.
//   - *relaymodel.Usage: the measured or explicitly estimated usage.
func (o *Observer) Finish(disconnected bool, streamErr error) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	o.Fail(streamErr)
	if disconnected {
		o.Fail(context.Canceled)
	}
	if !o.receipt || !o.stopped {
		o.Fail(errors.New("incomplete AWS response stream"))
	}
	o.snapshot()
	if o.err == nil {
		return nil, o.usage
	}
	code, status := "aws_stream_incomplete", http.StatusBadGateway
	if errors.Is(o.err, streaming.ErrQuotaExceeded) {
		code, status = "insufficient_user_quota", http.StatusForbidden
	}
	if o.failureFromUpstream(disconnected) {
		return openai.UpstreamErrorWrapper(o.err, code, status), o.usage
	}
	return openai.ErrorWrapper(o.err, code, status), o.usage
}
