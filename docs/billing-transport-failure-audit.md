# Undispatched billing failures and cancellation

## Reproduction boundary

The regressions exercise the actual DeepSeek adaptor and outer Relay retry loop
with fake loopback upstreams, opaque synthetic transports, and a disposable
SQLite database. No live provider, customer account, or production ledger is
involved. Fixture tariffs use the existing channel ratio contract and a
successful synthetic receipt settles exactly 15 quota units.

At base `aeb09a014dcbef23495da75070228e8701ed18d0`, the focused suite exited 1:
12 leaf scenarios failed and 8 controls passed. Two defects were demonstrated:

- Invalid URL/header preparation refunded the physical reservation through the
  audit fallback but left its positive type-6 provisional row unreconciled,
  including after a selected retry. Retry balances themselves were correct.
- Cancellation after reservation but before dispatch retained the synthetic
  1120-unit hold. A custom transport that ignores context was also invoked.

The repaired focused suite exits 0: all 20 leaf scenarios pass across 6
top-level tests, with no skipped cases.

The complete affected-package race suites also pass: 769 top-level tests,
zero failures and zero skipped cases. The full repository/database qualification
is not represented by this affected-package result.

Go 1.27.2 qualification also passed `go vet -p 1 ./...` and
`make test-quick`. Validation was serialized after the concurrent frontend
workstream completed its heavy checks.

## Repair and preserved behavior

The REST helper checks cancellation before logging/marking possible forwarding
or invoking the client. It closes the owned outgoing body and preserves both
cancellation and any close error. An existing forwarding marker is untouched.

The text controller routes already-known unforwarded request failures through
the existing detached, bounded refund lifecycle. That lifecycle owns balance
refund and provisional-row reconciliation before the deferred audit can launch
an emergency refund. Existing ambiguous-outcome handling stays in place.

Controls preserve terminal holds and audit signals for opaque transports,
custom dial failures, an upstream that reads the body then closes, and a written
redirect followed by dial failure. A selected retry refunds/voids its abandoned
attempt and leaves only the successful receipt's charge. Preparation/cancellation
refunds never over-credit user/token balances.

A custom dialer returning no connection in a fixture is not a general production
proof. Alternate transports, callbacks, redirects and internal retries can run
work outside standard trace callbacks. The fix does not classify those outcomes
from error shape or absent callbacks.

## Run the focused regression

```sh
go test -race -p 1 -run '^TestBillingTransport' -count=1 -timeout=120s ./relay/adaptor ./relay/controller
```

Test files:

- `relay/adaptor/billing_transport_precancel_test.go`
- `relay/controller/billing_transport_audit_integration_test.go`

## Evidence limits and follow-up

Configured `APP_LOG_SINK=stdout,otlp` bypasses the rotating file sink; see
`common/logger/logger.go` and `common/config/observability_app_log_otlp.go`.
Source history supports a logging configuration cutover, but does not prove its
exact runtime activation or telemetry delivery.

These source regressions do not establish the final charge of a historical
request. An audit alert precedes the outer retry decision; later refunds and
settlement require durable correlation. Missing logs and provisional request-cost
estimates are insufficient evidence of a terminal financial outcome.

[Issue #562](https://github.com/Laisky/one-api/issues/562) tracks read-only
log/trace/ledger correlation, delivery/retention limits, failed durable-operation
visibility, and any future trusted dispatch-evidence contract.
