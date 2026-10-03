# PR #440 non-adaptor scope and correctness audit

Audit date: 2026-10-02 (America/Toronto). This report supersedes earlier broad
claims that every inherited cross-cutting change was required.

## Scope and provenance

The resumed reviewed head is `65269523add403b25c5e00cf501eb9eac8f13d9a`
(tree `8fc2e194969c381ca00d66c5009a3ed8f9ba2b67`). The PR merge base is
`b1a2273ef41d4375ffe7bc5f18888ce11356285c`, not the advancing main tip
`d7fa5f35ebf3dd6072eace68bd0178196baa0823`. Comparing the two branch tips
would incorrectly attribute unrelated main changes to this PR.

The starting PR has 103 changed files. Outside `relay/adaptor/muapi/` there
are **86 files: 44 production, 36 test, and six documentation files**.
Every original file is listed below, including files restored to the merge base.
The follow-up appends changes; contributor ancestry is not rewritten.

## Findings and disposition

1. **Legacy request compatibility:** the early replay middleware applied the
   new JSON-only, 1-MiB hashing contract to traditional form/multipart/large JSON
   video requests merely because they included an idempotency key. Look up an
   owner-scoped receipt first. Only existing durable tasks use that hash check;
   new durable admissions still enforce their own bounds and full debit.
2. **Unnecessary second recovery system:** legacy bindings were extended with
   a retry table, a global pending map, detached retry goroutines and startup
   replay. The new durable worker never depends on them. A fault confined to
   that retry table also blocked healthy legacy GET/DELETE operations. Restore
   the legacy binding and OpenAI/xAI implementations exactly to the merge base,
   remove that table's migration/startup replay, and remove only its obsolete
   feature-specific tests. Keep all applicable original legacy and new durable
   billing tests. Existing deployments may leave the unused retry table in place;
   this patch does not drop data or claim to recover its historical pending rows.
3. **Unnecessary legacy pricing hook:** MuAPI implements the durable Provider
   contract and is rejected by the old video controller. Its estimator chain
   there is unreachable. Remove it; retain only the protocol bypass guard.
4. **Premature public output:** a valid running observation could carry a video
   URL and the worker/serializer exposed it before final settlement. The worker
   now stores output only on completion; serialization independently requires
   both `completed` and `settled`. A real second-provider preview/final-cost test
   proves a supplemental debit precedes final output. Synthetic invalid stored
   state combinations are defense-in-depth tests, not public exploit claims.
5. **Quote parser boundary:** the shared decimal helper used `big.Rat.SetString`
   without bounding grammar/length/exponents, accepting fractions and hex floats.
   Reuse the bounded USD parser before arbitrary precision work. Apply the same
   bound before the adapter's earlier parse so checking only downstream is not
   a false fix. Tiny supported decimals still round upward correctly.
6. **Identity survives malformed cost:** the evidence helper rejected an invalid
   amount before saving an otherwise valid accepted ID. Persist identity alone,
   keep the accounting error/hold, and recover the same accepted task. This is a
   common-provider invariant even though MuAPI already validates its cost fields.
7. **Receipt retention progress:** a full 100-row prefix of old but unresolved
   receipts monopolized every cleanup batch. Persist a deferred existence-check
   deadline for live tasks; later orphan receipts can be pruned while all held
   financial records remain. This does not refund or alter wallet balances.

The new negative controls compiled on unchanged `6526952` production code:
**23 failing leaf cases and two passing controls** (20+2+1 failures across three
runs). These are parameterized assertions, not 23 independent vulnerabilities.
Safe-sized parser inputs were used in the negative control; no enormous exponent
allocation or real paid provider request was attempted.

## Original production files: individual necessity and correctness review

| File | Disposition | Necessity / scope boundary | Validation |
|---|---|---|---|
| `common/config/async_video.go` | Keep | Bound the HTTP waiter and number of provider workers independently; neither setting owns billing. | Wait-timeout/disconnect and worker shutdown tests; clamps inspected. |
| `controller/relay.go` | Keep; audit | Select the durable bridge only for registered async providers; irrevocable paid-replay veto must outrank 413 retry budgets. Apply endpoint compatibility to every retry candidate. | TestAsyncVideo413CannotReopenPaidReplay; text-capacity controls; shipped-router compatibility; unfiltered controller suite. |
| `controller/relay_retry.go` | Keep | Transport may already have forwarded any video POST, not only a hard-coded provider list. Reservations and accepted receipts forbid automatic paid replay. | TestRelayDoesNotReplayPossiblyForwardedPaidVideo; TestAsyncVideoSubmissionRetryBoundary; 413 controls. |
| `main.go` | Narrow | Start/join the durable worker only after database and pricing setup. Remove the unrelated legacy binding startup replay loop. | Worker shutdown and SIGKILL recovery; startup ordering and existing shutdown join inspected. |
| `middleware/auth.go` | Keep | Permit exhausted credentials to read/reattach prepaid work, while explicit disablement, expiry, owner status and model permissions remain enforced. New work still requires the database reservation. | Shipped-router exhausted/disabled credential cases; idempotency ownership/model-permission cases; existing auth suite. |
| `middleware/distributor.go` | Keep | Distinguish native capability from public delivery. Only the explicit generations POST bridges native async channels; legacy CRUD never does. | Native-capability isolation; shipped router selecting lower-priority compatible legacy channel; retry compatibility. |
| `middleware/video_task_binding.go` | Restore merge base | New durable reads have their own route/store. Unconditional dependency on the newly introduced legacy retry table broke healthy legacy GET/DELETE requests. | TestAsyncVideoExistingBindingDoesNotDependOnRetryStore ran red on the original PR and green after restoration; original binding tests retained. |
| `model/async_jobs.go` | Keep; correct | Task plus full debit must commit together; owner UUID and dedup/hash distinguish paid retries from new work. Add owner-scoped receipt lookup before parsing legacy request bodies. | Reservation rollback, concurrent admission, lost COMMIT, UUID reuse, and TestAsyncVideoReplayPreservesLegacyPayloads. |
| `model/async_jobs_evidence.go` | Keep; correct | Persist accepted identity and monotonic costs separately from the financial transaction. A malformed cost must not erase an independently valid provider ID. | TestAsyncBillingInvalidCostKeepsAcceptedIdentity; real-backend variant; existing evidence rollback/lost-COMMIT tests. |
| `model/async_jobs_lifecycle.go` | Keep | Fenced leases, byte-exact accepted identity, atomic supplement/settlement/refund, and retention of outstanding holds belong to the shared store, not an adapter. | Financial state matrix; competing closures; SQL-failure/commit-loss cases; nine SIGKILL boundaries; actual SQL identity tests. |
| `model/async_jobs_log.go` | Keep; correct | Split databases require versioned receipts/outbox. Independent backoff is necessary. Defer cleanup checks of live old receipts so a 100-row prefix cannot starve later orphan cleanup. | Outbox races/poison batches; TestAsyncBillingReceiptRetentionSkipsLivePrefix; actual SQL retention variant. |
| `model/async_jobs_price.go` | Keep | Capture conversion once, validate bounded decimal evidence, round upward, and collect only the monotonic supplement, including debt. | Cost adjustment/numeric bounds/evidence suites; MySQL/PostgreSQL finite and unlimited token cases. |
| `model/async_jobs_quota.go` | Keep | Owner and token balance changes must be in the task transaction, with immutable identity, policy and numeric fences. | Concurrent distinct admissions; token deletion/reuse; finite/unlimited policy changes; refund races and rollback matrix. |
| `model/async_task.go` | Restore merge base | Legacy bindings are not the durable async_tasks ledger. The added retry table/API was a second recovery mechanism unnecessary for the new bridge. | Byte-identical to merge base; original binding and retention tests; new durable crash matrix remains. |
| `model/main.go` | Narrow | Migrate async_tasks in the primary DB and its versioned log receipts in the actual log DB. Remove the unnecessary legacy retry-table migration. | Payload dialect tests; actual disposable primary/log database acceptance; migration wiring inspected. |
| `model/token.go` | Keep | Share credential validation while separating paid-task retrieval from fresh positive-balance admission. Do not fork or weaken normal token checks. | Existing model/token and middleware suites; spent-token reattachment and disabled-token denial through the real router. |
| `relay/adaptor.go` | Keep | Register construction of the MuAPI adapter; no default fall-through or modification of existing provider selection. | TestMuAPIRegistration; relay registry suite. |
| `relay/adaptor/interface.go` | Keep | Represent a whole-request decimal quote without per-second float round trips; copy new quote fields in HasData/Clone. | Exact decimal quota controls; existing pricing-copy and time-window tests. |
| `relay/adaptor/openai/video.go` | Restore merge base | OpenAI response handling need not change for a MuAPI worker-backed bridge. Remove the inherited error-signature/logging expansion. | Byte-identical to merge base; unfiltered OpenAI suite and shipped traditional video path. |
| `relay/adaptor/video.go` | Keep | Use optional request-specific pricing interfaces rather than provider switches in billing. Existing adapters are not required to implement them. | MuAPI quote fail-closed tests; whole-catalog pricing audit; interface callers inspected. |
| `relay/adaptor/xai/video_response.go` | Restore merge base | The xAI protocol/accepted-task handling is independent of MuAPI durable dispatch; do not expand its binding failure behavior here. | Byte-identical to merge base; unfiltered xAI tests and original xAI controller tests. |
| `relay/apitype/define.go` | Keep | Append, never renumber, the provider API type. | Registry tests and enum append-only diff inspection. |
| `relay/apitype/helper.go` | Keep | Give the appended API type its stable name. | TestMuAPIRegistration. |
| `relay/asyncvideo/binding.go` | Restore merge base | Remove an unbounded in-memory pending map and detached per-request retries that are not used by the durable worker. Preserve legacy binding implementation unchanged. | Byte-identical to merge base; no references to the removed retry API remain; legacy and durable suites. |
| `relay/asyncvideo/observation.go` | Keep | Enforce normalized state/refund/result constraints for every worker provider, including safe bounded result URLs. | Second-provider contract and malformed-poll tests; real partial-result regression. |
| `relay/asyncvideo/provider.go` | Keep | Define submit receipt, poll observation and public video result types without HTTP writes or financial side effects in providers. | Second-provider worker/DTO test; implementations and call graph inspected. Full admission still requires registry/pricing/preparer integration. |
| `relay/asyncvideo/worker.go` | Keep; correct | Joined, bounded workers resume GET polling and hold uncertain POST outcomes. Store output only for completed observations, not running previews. | TestAsyncVideoWorkerWithholdsPartialResults; shutdown/disconnect/crash tests; no per-request worker goroutine. |
| `relay/channeltype/async_video.go` | Keep | One explicit native-capability registry determines whether the generic bridge may be selected. | Native-capability isolation; real first/retry routing; extension contract inspected. |
| `relay/channeltype/define.go` | Keep | Append channel ID 61 without shifting existing persistent channel IDs. | TestMuAPIRegistration pins TypeSafe=60 and MuAPI=61. |
| `relay/channeltype/endpoints.go` | Keep | Public async API and MuAPI native protocol need distinct IDs; MuAPI defaults must not advertise legacy videos. | Endpoint defaults/isolation tests and request-aware distributor cases. |
| `relay/channeltype/helper.go` | Keep | Map the appended channel to its provider API type and display name. | Registry and channeltype tests. |
| `relay/channeltype/url.go` | Keep | Supply the registered default origin and editable proxy behavior at the correct numeric index. | Registry array length/default tests; typed quote/proxy origin controls. |
| `relay/controller/async_video.go` | Keep; correct | Shared admission, read, idempotent reattachment and bounded waiting are gateway responsibilities. Check for an existing receipt before applying its body contract, and expose output only after completed+settled. | Legacy form/multipart/large JSON red controls; serializer state/billing matrix; real worker preview test; owner and paid-wait tests. |
| `relay/controller/async_video_pricing.go` | Keep | Resolve administrator tariffs first, otherwise request an exact provider quote and capture its conversion before task admission. | Quote failures cannot dispatch paid work; existing override/quota tests; exact bounded decimal controls. |
| `relay/controller/async_video_provider.go` | Keep | Reload current credentials only for the stored immutable channel identity/type/base; never store provider keys in tasks or fail over accepted work. | Disabled-channel polling, route/UUID fences and restart recovery; resolver code inspected. |
| `relay/controller/video.go` | Narrow | Keep only the guard preventing native providers from bypassing durable dispatch. Remove the unreachable MuAPI estimator/total-quote path from the legacy controller. | Legacy sync/form/large-body controls; existing video/xAI suites; durable routed admissions unaffected. |
| `relay/controller/video_quota.go` | Keep; correct | A provider total needs exact upward quota conversion. Reuse bounded decimal validation before big-number parsing, rather than accepting fractions/hex/excess exponents. | TestVideoTotalQuoteHasBoundedDecimalSyntax plus exact/tiny/free-group/overflow controls; adapter validates before its own parse too. |
| `relay/pricing/timewindow.go` | Keep | When shared pricing types gain two quote representations, replacing either must clear the stale inherited representation. | TestMergeVideoTotalPricingReplacesBothRepresentations; full pricing suite. |
| `relay/relaymode/define.go` | Keep | Append public/native async modes, preserving historical enum IDs and names. | Registry/protocol tests and append-only diff inspection. |
| `relay/relaymode/helper.go` | Keep | Map explicit async routes without changing legacy videos classification. | Route-isolation tests and shipped router. |
| `router/relay.go` | Keep | Register durable create/read and replay before new channel admission. The global limiter must cover early reattachment and run before Authorization is rewritten to a provider key. | TestAsyncVideoGlobalRateLimitCoversPrepaidReads; shipped-router routes, auth, ownership, quota and disconnect tests. |
| `web/air/src/constants/channel.constants.js` | Keep | Make the appended backend channel selectable without changing existing options. | Modern metadata mirror test; existing Air CI tests/build on unchanged frontend content. |
| `web/berry/src/constants/ChannelConstants.js` | Keep | Mirror channel ID/name for the supported legacy theme. | Modern metadata mirror test; existing Berry CI tests/build on unchanged frontend content. |
| `web/modern/src/pages/channels/constants.ts` | Keep | Expose correct native capability, bridge paths and dynamic-price behavior in the primary UI. | MuAPI metadata test; unchanged frontend content compared with already-qualified head; ordinary final CI tracked separately. |

## Original test files: individual disposition

Restoring tests for a removed optional subsystem is not equivalent to weakening
billing regressions. All durable transaction, HTTP disconnect, SIGKILL, identity,
refund, supplement and outbox assertions remain. The removed MuAPI-only legacy
binding retry test is inside the adapter directory and is recorded separately.

| File | Disposition / purpose |
|---|---|
| `controller/async_video_413_regression_test.go` | Retain regression coverage. Entrypoints: `TestAsyncVideo413CannotReopenPaidReplay`, `TestAsyncVideo413PreservesTextCapacityRecovery`. |
| `controller/async_video_retry_regression_test.go` | Retain regression coverage. Entrypoints: `TestAsyncVideoSubmissionRetryBoundary`, `TestAsyncVideoTransientClientErrorCannotOverrideReplayVeto`. |
| `controller/live_transport_retry_test.go` | Retain regression coverage. Entrypoints: `TestRelayLiveOnlyRESTMismatchFallsBackToCompatibleBridge`, `TestRelayLiveOnlyRESTMismatchReturns400WhenNoBridgeExists`, `TestRelayDoesNotReplayPossiblyForwardedPaidVideo`. |
| `controller/relay_retry_priority_order_test.go` | Retain regression coverage. Entrypoints: `TestRelayRetry_429StrictPriorityOrder`, `TestRelayRetry_429NextTierSucceeds`, `TestRelayRetry_429SameTierSiblingBeforeLowerTier`, `TestRelayRetry_5xxStrictPriorityOrder`; additional existing tests retained. |
| `controller/xai_video_retry_test.go` | Retain regression coverage. Entrypoints: `TestXAIVideoRetrySafety`. |
| `middleware/video_task_binding_test.go` | Restore original merge-base tests; remove only assumptions/tests for the removed legacy retry subsystem.Entrypoints: `TestBindAsyncTaskChannelSetsContext`, `TestBindAsyncTaskChannelNoRecord`. |
| `model/async_billing_ack_backoff_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingAckBatchBackoff`, `TestAsyncBillingAckBackoffLiveDatabases`. |
| `model/async_billing_commit_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingLostCommitAcknowledgements`. |
| `model/async_billing_matrix_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingStateMatrix`, `TestAsyncBillingConcurrentDifferentRequestsCannotOverspend`, `TestAsyncBillingConcurrentSettlementAndRefundHasSingleWinner`, `TestAsyncBillingCostAdjustmentsAreMonotonic`; additional existing tests retained. |
| `model/async_billing_multidb_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingLiveDatabases`. |
| `model/async_billing_outbox_race_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingStaleOutboxCannotOverwriteFinalChargeOrRefund`, `TestAsyncBillingWholePoisonBatchBacksOff`, `TestAsyncBillingNumericBounds`. |
| `model/async_billing_outbox_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingPoisonOutboxDoesNotBlockLaterReceipts`, `TestAsyncBillingSettlementUsesPersistedFinancialSnapshot`, `TestAsyncBillingHeldTaskHasDurableCost`. |
| `model/async_jobs_evidence_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingEvidenceRecovery`, `TestAsyncBillingEvidenceLiveDatabases`, `TestAsyncBillingEvidenceLostCommitAcknowledgement`. |
| `model/async_jobs_identity_regression_test.go` | Retain regression coverage. Entrypoints: `TestAsyncTaskSettlementCannotReplaceAcceptedIdentity`, `TestAsyncBillingIdentityLiveDatabases`. |
| `model/async_jobs_schema_test.go` | Retain regression coverage. Entrypoints: `TestAsyncTaskPayloadColumnCapacity`. |
| `model/async_jobs_test.go` | Retain regression coverage. Entrypoints: `TestAsyncTaskRestartResumesWithoutAnotherDebit`, `TestAsyncTaskAdmissionRollbackCannotLeaveAnOrphan`, `TestAsyncTaskSettlementFailureIsAtomic`, `TestAsyncTaskSplitLogOutboxIsIdempotent`; additional existing tests retained. |
| `model/async_task_test.go` | Restore original merge-base tests; remove only assumptions/tests for the removed legacy retry subsystem.Entrypoints: `TestSaveAndGetAsyncTaskBinding`, `TestTouchAsyncTaskBinding`, `TestCleanExpiredAsyncTaskBindings`. |
| `relay/asyncvideo/async_billing_process_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingProcessChild`, `TestAsyncBillingSIGKILLRecovery`. |
| `relay/channeltype/async_video_protocol_regression_test.go` | Retain regression coverage. Entrypoints: `TestMuAPINativeVideoCapabilityIsIsolated`. |
| `relay/channeltype/muapi_test.go` | Retain regression coverage. Entrypoints: `TestMuAPIRegistration`. |
| `relay/controller/async_billing_recovery_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingTemporaryResolverFailureResumesPolling`, `TestAsyncBillingMalformedOutputRetainsObservedCost`, `TestAsyncBillingUnknownSubmissionStillCollectsKnownCost`, `TestAsyncBillingSupplementFailureWithholdsResult`; additional existing tests retained. |
| `relay/controller/async_billing_shutdown_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingShutdownPreservesReceipt`, `TestAsyncBillingActualProviderChargeCannotExceedDebit`. |
| `relay/controller/async_video_behavior_test.go` | Retain regression coverage. Entrypoints: `TestAsyncVideoLostAcknowledgementNeverReplays`, `TestAsyncVideoCrashFencesSubmitAndRecoversPolling`, `TestAsyncVideoRefundRequiresProviderConfirmation`, `TestAsyncVideoWaitTimeoutDisconnectAndSyncResult`; additional existing tests retained. |
| `relay/controller/async_video_provider_contract_test.go` | Retain regression coverage. Entrypoints: `TestAsyncVideoSecondProviderUsesSameLifecycle`, `TestAsyncVideoPersistenceFailureAfterAcceptance`. |
| `relay/controller/muapi_quote_channel_test.go` | Retain regression coverage. Entrypoints: `TestMuAPIQuoteChannelMismatchCannotReserveWork`. |
| `relay/controller/muapi_video_test.go` | Retain regression coverage. Entrypoints: `TestMuAPIVideoLifecycle`, `TestMuAPIVideoLifecycleFailsClosedBeforeSubmission`, `TestMuAPIVideoAcceptedDisconnectKeepsChargeAndBinding`. |
| `relay/controller/video_quota_test.go` | Retain regression coverage. Entrypoints: `TestVideoQuotaDecimal`. |
| `relay/controller/xai_video_test.go` | Restore original merge-base tests; remove only assumptions/tests for the removed legacy retry subsystem.Entrypoints: `TestXAIVideoBillingHTTP`, `TestXAIVideoAdmissionRejectsUnpricedWork`, `TestXAIVideoAcceptedDisconnect`. |
| `relay/muapi_test.go` | Retain regression coverage. Entrypoints: `TestMuAPIRegistration`. |
| `relay/pricing/timewindow_test.go` | Retain regression coverage. Entrypoints: `TestMatchTimeWindowScheduleSemantics`, `TestApplyTimeWindowPrecedenceAndScalarMerge`, `TestApplyTimeWindowTierMerge`, `TestApplyTimeWindowNestedPricingMerge`; additional existing tests retained. |
| `router/async_billing_disconnect_test.go` | Retain all behavior assertions; stop migrating the removed retry table in the fixture. Entrypoints: `TestAsyncBillingRealSocketDisconnect`. |
| `router/async_billing_headers_test.go` | Retain regression coverage. Entrypoints: `TestAsyncBillingHeaderChargeReachesWallet`. |
| `router/async_video_admission_regression_test.go` | Retain regression coverage. Entrypoints: `TestAsyncVideoRejectedAdmissionHasNoPhantomTask`. |
| `router/async_video_test.go` | Retain all behavior assertions; stop migrating the removed retry table in the fixture. Entrypoints: `TestAsyncVideoShippedRouterCompatibility`. |
| `test/adaptor_pricing_wiring_test.go` | Retain regression coverage. Entrypoints: `TestEveryAdvertisedModelIsPricedByItsChannel`, `TestCompatibleChannelPricingMatchesAdvertisedCatalog`, `TestNoPerTokenPriceLooksLikeAUnitError`, `TestSameVendorTablesAgreeOnSharedModels`; additional existing tests retained. |
| `web/modern/src/pages/channels/__tests__/muapi-constants.test.ts` | Retain regression coverage. Validate channel ID 61 and theme mirrors; no frontend behavior is removed. |

## Original documentation files: individual disposition

| File | Necessity / interpretation |
|---|---|
| `docs/muapi-video.md` | Current operator/API contract; update scope notes for removal of the legacy retry subsystem and reinforce final-result/receipt rules. |
| `docs/research/20260924_adaptor_model_catalog_audit.md` | Catalog audit records the added provider disposition; keep append-only evidence. |
| `docs/research/20261002_async_video_billing_fault_matrix.md` | Historical executable financial matrix remains useful; this audit adds rather than replaces its applicable cases. |
| `docs/research/20261002_async_video_validation.md` | Original implementation and red/green provenance; historical, not a claim that the optional legacy retry subsystem remains shipped. |
| `docs/research/20261002_pr440_final_review.md` | Preserve prior 413/admission/identity corrections and their evidence; this audit is a later scope review. |
| `docs/research/20261002_pr440_review_closure.md` | Preserve prior outbox and typed-origin CodeQL evidence; final-head CI and scan qualifications remain distinct. |

## New regression files introduced by this audit

| File | What it proves |
|---|---|
| `router/async_video_legacy_payload_test.go` | Real synchronous provider keeps identical form, multipart, large JSON and ordinary JSON behavior with/without an idempotency header. No async task is fabricated. |
| `middleware/async_video_existing_binding_test.go` | Healthy legacy GET/DELETE never read the optional retry table. |
| `relay/controller/async_video_public_boundary_test.go` | Public result/billing matrix and bounded decimal grammar with positive exact/tiny controls. |
| `relay/controller/async_video_partial_result_test.go` | Real reservation/worker/settlement hides running previews and collects the larger final charge before output. |
| `model/async_billing_invalid_evidence_test.go` | Malformed cost retains valid identity; same-job recovery and one supplement, locally and on actual SQL engines. |
| `model/async_billing_retention_progress_test.go` | Bounded cleanup passes 100 old live receipts without deleting unresolved funds, locally and on actual SQL engines. |
| `router/async_video_rate_boundary_test.go` | Early reads/replays are rate-limited per original client credential; a different token is not incorrectly grouped with the upstream key. |

## Verification gates

The exact final source tree and actual command outcomes are recorded in the PR
acceptance comment and downloadable evidence. Do not infer a completed final CI
run merely from an earlier head's successful run.

```sh
go vet ./...
go test -json -race -count=1 -p 2 -timeout 15m \
  ./relay/adaptor/muapi ./relay/adaptor/openai ./relay/adaptor/xai \
  ./relay/asyncvideo ./relay/channeltype ./relay/relaymode ./relay/pricing \
  ./relay ./middleware ./controller ./relay/controller ./router
go test -json -race -count=1 -timeout 12m \
  -run 'TestAsync|TestMuAPI|TestVideoQuota' ./model
go test -json -race -count=25 -p 2 -timeout 8m \
  -run 'TestAsyncVideoReplayPreservesLegacyPayloads|TestAsyncVideoExistingBindingDoesNotDependOnRetryStore|TestAsyncVideoPublicResultRequiresSettlement|TestVideoTotalQuoteHasBoundedDecimalSyntax|TestAsyncVideoWorkerWithholdsPartialResults|TestAsyncBillingInvalidCostKeepsAcceptedIdentity|TestAsyncBillingReceiptRetentionSkipsLivePrefix|TestAsyncVideoGlobalRateLimitCoversPrepaidReads' \
  ./model ./middleware ./relay/controller ./router
ONEAPI_REQUIRE_DB_BACKENDS=1 go test -json -race -count=1 -timeout 8m \
  -run '^TestAsyncBilling(LiveDatabases|EvidenceLiveDatabases|AckBackoffLiveDatabases|IdentityLiveDatabases|InvalidEvidenceLiveDatabases|RetentionLiveDatabases)$' ./model
```

The actual backend gate requires **22 named leaf cases with zero skips** across
MySQL and PostgreSQL. Missing local DSNs are skips, not passes. The SIGKILL child
entrypoint intentionally skips in the parent process; all nine actual crash
scenarios must execute. The frontend files themselves are unchanged by this
audit, but ordinary final-head CI remains the integration/build gate.

## Limits and operational implications

The ledger and joined worker are essential shared infrastructure; they cannot
be pushed into the MuAPI adapter without tying accounting to a client socket.
The audit does not claim provider-side exactly-once, collection of an invoice the
provider never exposed, catastrophic-database-loss recovery, or a reconciliation
administration UI. Missing receipts remain held and explicit.

This patch removes only an unneeded secondary *legacy binding* recovery system,
not `async_tasks`, its financial evidence/outbox, or background task execution.
Existing legacy provider behavior is restored, not retroactively made durable.
For an experimental deployment of an earlier PR revision, inspect/preserve any
`async_task_binding_retries` rows before switching versions; there is no automatic
migration of those optional legacy rows into the financially different new ledger.

Synchronous waiting still uses bounded periodic database reads (250 ms per
waiter), and idle workers scan for durable work. This review verifies correctness
and bounded work, not high-scale latency/throughput. A thousand simultaneous
waiters would imply roughly 4,000 status reads/second before completions; use the
explicit async API or measure capacity before claiming such scale. No performance
SLA or claim of globally optimal scheduling follows from passing behavior tests.
