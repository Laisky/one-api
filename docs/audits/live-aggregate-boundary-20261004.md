# Live aggregate transport boundary: partial mitigation for #462

Status: partial mitigation; **#462 remains open**. This change does not establish
that provider spend is prepaid. It adds no prices, disables no model or modality,
and changes no reservation or settlement amount. No paid provider was contacted.

## Enforced behavior

Developer API and Vertex use the same `LiveHandlerWithTransport` and pump.
A session may forward at most **8 MiB of encoded client JSON** and **32,768
client data frames**, including the rewritten setup. Text, PCM audio, images,
video frames, client history, function responses, activity controls and empty
operations share this allowance. The existing 4 MiB per-message limit remains.
The allowance is independent of session duration and cannot reset on a receipt,
turn completion or function call. Failed writes do not restore allowance.

The complete next frame must fit before any of its bytes are written upstream.
An excess frame receives WebSocket policy close reason
`gemini_live_input_budget_exhausted`. The existing bounded receipt drain and
settlement continue for already forwarded input. A rejected frame does not add
fake usage or mark an otherwise complete receipt ledger incomplete. Debug logs
report only aggregate bytes, frames and exhaustion status, never frame contents.

The finite input budget can close an active audio session before the existing
15-minute lifetime. Eight MiB includes base64 expansion and JSON overhead; it is
not eight MiB of decoded audio. Clients must handle the explicit policy close.
Each new session still passes normal authentication and quota admission, but
reconnecting can obtain another resource allowance if admission succeeds.

## Behavioral evidence

The test-only RED commit `929ca52b` ran
`go test ./relay/adaptor/gemini -run TestGeminiLiveAggregateInputBoundary -count=1`
against base `952ae6d2`. Three actual loopback WebSocket cases each forwarded
**nine** one-MiB text, PCM audio or JPEG envelopes where the new allowance should
permit **seven** after setup. JPEG data is a genuinely encoded image; remaining
JSON whitespace pads the exact resource-boundary test size. This is evidence
about bytes forwarded, not about a provider tokenization rate.

After the fix those same cases forward seven envelopes. Additional socket
regressions cover requested function results, tiny activity-control floods, and
pending usage after the provider omits receipts. Exact-boundary arithmetic and
terminal exhaustion are covered separately. Existing positive controls exercise
bidirectional audio/transcripts, function cancellation/results, Extended Thinking,
late receipts, configured models, and Developer/Vertex transport behavior.

The existing accounting and contention tests are supporting evidence only:
`TestGeminiLiveDurableAccounting` checks SQLite user/token/channel balances and
consume-log metadata for complete, partial, missing, idle and free-group usage;
`TestBillingAuditConcurrentAdmission` checks atomic user/token reservation.
Neither test proves a session-wide prepaid monetary ceiling.

## Why this does not close the monetary finding

Google's [Live billing documentation](https://ai.google.dev/gemini-api/docs/live-api/best-practices)
says context is charged again on later turns, transcripts incur additional text
output charges, and 3.8 Live models permanently enable proactive audio. Those
facts prevent a raw byte limit or a duration allowance from directly representing
all billable work. A native input can cause output and context reprocessing after
the proxy has accepted it. Closing a socket after observing usage is too late to
reserve quota for that work.

The [Live API reference](https://ai.google.dev/api/live) exposes generation
configuration, asynchronous realtime input and tool responses. Its linked
[generation configuration reference](https://ai.google.dev/api/generate-content#v1beta.GenerationConfig)
describes `maxOutputTokens` in terms of response candidates. This review did not
establish an exact, provider-enforced total ceiling covering every supported Live
model's audio, thinking, transcripts and autonomous continuations. That is an
explicit evidence gap, not a claim that Google cannot offer such a contract.

## Required remaining implementation and acceptance

1. Establish model/backend-specific enforceable bounds for input tokenization,
   each output modality, reasoning, transcription, context re-billing and possible
   autonomous/background turns. Include permitted function results and interruption
   behavior. Reject an operation whose cost contract is unknown before forwarding.
2. Price those bounds through the existing configured tariff resolver, including
   tiers and genuine free-group policy. Do not borrow Developer prices for Vertex
   or unlisted administrator-configured models.
3. Share one session reservation ledger across setup, every potentially billable
   dispatch and settlement. Atomically reserve additional user and finite-token
   quota before forwarding, with durable crash/retry and exactly-once settlement
   semantics. Preserve unknown usage for reconciliation and never refund incurred
   cost merely because a receipt is missing.
4. Exercise the production controller through loopback sockets and a real test
   database. Assert provider-received work, simultaneous same-user/token sessions,
   balances and durable logs for exhaustion, delayed/duplicate/partial/missing
   receipts, disconnect and timeout. The current resource tests cannot substitute
   for these monetary acceptance cases.

Until that contract and acceptance evidence exist, the short audio admission
reservation is still an estimate. Complete receipts can establish debt; missing
receipts retain a lower-bound estimate. This PR must not be described as fixing
unfunded Live spending or automatically closing #462.

## Validation at this checkpoint

- `go vet ./...` passed; the final fixture-only deadline adjustment also passed
  `go vet ./relay/adaptor/gemini ./relay/adaptor/vertexai`.
- `go test -race ./relay/adaptor/gemini ./relay/adaptor/vertexai ./relay/realtime ./relay/quota -run 'Test.*(Live|Gemini)' -count=1` passed.
- `go test -race ./controller ./model -run 'Test(GeminiLiveDurableAccounting|GeminiReceiptSettlementBehavior|GeminiRealtimeChannelAndReservation|GeminiRejectedFinalReceiptRetainsSettlementFloor|BillingAuditConcurrentAdmission)' -count=1` passed.
- The initial race run reached the existing fixture's five-second socket timeout
  while validating multi-MiB JSON. The large-payload fixtures now use explicit
  one-minute bounded deadlines; production timeouts were not changed.
- Full repository race and Modern frontend checks remain the integration gate.
