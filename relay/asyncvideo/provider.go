package asyncvideo

import (
	"context"

	"github.com/Laisky/one-api/relay/meta"
)

// Provider is the upstream submit/poll contract. Implementations only perform
// provider I/O and normalization: no Gin response writes, database mutations,
// quota settlement or retry loops. This makes a second provider adapter-only.
type Provider interface {
	SubmitVideo(context.Context, *meta.Meta, []byte) (Submission, error)
	PollVideo(context.Context, *meta.Meta, string) (Observation, error)
}

// Submission distinguishes an explicit non-acceptance from an unknown outcome.
// On any error, the default Rejected=false MUST be treated as possibly accepted.
type Submission struct {
	ID       string
	Rejected bool
	CostUSD  string
}

// Video is one generated output. Provider metadata, cost and credentials are
// never forwarded wholesale into the public result.
type Video struct {
	URL string `json:"url"`
}

// Result is the provider-independent video payload, shared by task retrieval
// and successful synchronous responses.
type Result struct {
	Videos []Video `json:"videos"`
}

// Observation is an authoritative provider state. Failed work is refundable
// only after upstream confirmation, not because polling timed out or returned 5xx.
type Observation struct {
	State    string
	Result   *Result
	Refunded bool
	CostUSD  string
}

// DurableTaskKey marks an HTTP request already represented by a durable task.
// No automatic relay retry may create another job after this boundary.
const DurableTaskKey = "relay.durable_async_task"

// HandledResponseKey marks an async response already written locally without a
// durable task (for example, admission rejection). Relay must record the actual
// HTTP outcome without entering cross-channel retry or counting it as success.
const HandledResponseKey = "relay.async_video_handled_response"
