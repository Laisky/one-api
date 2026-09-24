# Reproduce the 2026-09-24 model snapshots

These scripts produce static deployment artifacts, not a runtime model scraper.
They use only archived official public responses and reject a source whose bytes
no longer match `sources.lock.json`. They never invoke inference, access channel
credentials, or change an administrator's channel configuration.

## Evidence

Download the `catalog-audit-public-evidence` artifact from
[run 36001316470](https://github.com/Laisky/one-api/actions/runs/36001316470).
Artifact ID: **10808950479**; retention ends **2026-12-23**. Verify its ZIP SHA-256:

```text
82b8b9bce5d5b0094a2854711f0458fd01b2a3411bfadd084514bd5268d5f29a
```

Extract the archive and use the evidence directory containing `first/`,
`second/`, and `final/`. Each provider's source bytes have a second, independent
SHA-256 check in the source lock. Archive credentials and signed download links
are not part of the repository. The source bytes should be retained in the
project's long-term evidence store before GitHub retention expires.

## Commands

Run from the repository root with Python 3.11+ and the Go version in `go.mod`:

```sh
python3 -m venv .venv-catalog
. .venv-catalog/bin/activate
python3 -m pip install -r scripts/model_catalog/requirements.txt
export MODEL_CATALOG_EVIDENCE=/absolute/path/to/catalog-evidence
python3 -m unittest discover -s scripts/model_catalog -v
python3 scripts/model_catalog/refresh.py --evidence-root "$MODEL_CATALOG_EVIDENCE"
gofmt -w relay/adaptor/*/zz_catalog_20260924.go
git diff --exit-code -- relay/adaptor/*/catalog_20260924.json \
  relay/adaptor/*/zz_catalog_20260924.go docs/research/model_catalog_20260924
go test -race -count=1 ./relay/adaptor/...
go vet ./relay/...
```

Without the environment variable, parser unit tests still run; archived-source
fixture tests require the evidence directory. Regeneration rewrites only dated
snapshots, their wrappers and reports. A changed source lock is an intentional
new audit, not a way to bypass a checksum failure.

## Units and exclusions

Snapshot `ratio` and positive cache prices are **native currency per million
tokens** until the Go loader converts them to internal quota units. Currency is
explicitly USD or CNY. Output is represented by its multiplier against the input
rate. A free cache read has the existing negative sentinel; missing cache data is
unknown, not free. Explicit zero input/output is accepted only for a quoted free
SKU. Per-character, duration, per-call, image, storage and deployment contracts
are not converted into token prices by this generator.

DeepInfra discounts are applied once to its native rate. Alibaba/China and
international tables remain separate. HTML row spans are expanded before
reading prices. Dayparts, date windows and inclusive/exclusive token boundaries
are provider-specific and covered by regression tests. Unknown, ambiguous,
batch-only and unsupported billing rows appear in each inclusion/exclusion
report rather than receiving a guessed price.

`catalogsnapshot.Apply` overlays only explicit fields onto cloned metadata. It
preserves existing IDs and unspecified fields; it is not a runtime request
allowlist. See the [47-surface ledger](../../docs/research/20260924_adaptor_model_catalog_audit.md)
for native-protocol exclusions and deployment-specific catalogs.
