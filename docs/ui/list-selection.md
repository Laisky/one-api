# Modern list selection

Channels, Users, Tokens, Redemptions, MCP Servers, and Logs share the same desktop/mobile selection controls.

- A row checkbox adds/removes that record by public UUID, never by page-local position.
- **Select this page** adds the visible page. The header checkbox can deselect only that page, preserving other selections.
- **Select all pages** selects every record matching the applied list scope, not unrelated filtered-out records. Unchecked records become explicit exclusions.
- The selection count and scope remain visible; cursor lists without an exact count say so. The server resolves the exact action targets before confirmation.
- Pagination, sorting, and page-size changes preserve selection. Filter or principal changes clear it and invalidate pending confirmations. Until edited filters are applied, stale displayed rows cannot be selected.
- The controls do not store credentials or whole records in browser persistence.

## Selected actions

Channel toolbar actions are **Test selected channels**, **Reset selected models**, and **Delete selected disabled channels**. They are disabled without a selection. Individual row actions remain available. Disabled-channel deletion checks each channel's current status inside its transaction and skips enabled channels.

Log actions are **Export selected logs** and **Delete selected logs**. Export enriches only selected rows with trace data. Deletion requires administrator access. Ordinary users can resolve/export only their own logs, regardless of UUIDs submitted by the browser.

Add, Search, Refresh, sorting, and statistics are view/creation controls, not record mutations; they retain their existing purpose. This change does not introduce new bulk account or token deletion capabilities. Legacy frontend routes remain compatible.

## API contract

Resolve a channel selection with `POST /api/channel/selection`:

```json
{"keyword":"provider-","selection":{"mode":"all_matching","excluded_ids":["018fcf6d-c484-7000-8000-000000000101"]}}
```

Or provide `{"selection":{"mode":"ids","ids":["018fcf6d-c484-7000-8000-000000000102"]}}` for explicit rows. The resolver uses the same prefix-name/exact-UUID predicate as the channel list.

Before a channel reset or disabled-channel deletion, the UI resolves this selection to a fixed UUID snapshot and confirms its actual count. It then sends **only** `selection.mode=ids` to `/api/channel/reset_models` or `/api/channel/delete_selected_disabled`. Missing selections, empty ID sets, malformed UUIDs, ambiguous modes, and all-matching mutation requests are rejected. New records created after confirmation cannot join the operation. Each channel is atomic; the persistent report distinguishes success, rejection/failure, and skips.

`POST /api/log/selection` similarly accepts `selection` and the list's `keyword`, `type`, `model_name`, `token_name`, `username`, `channel`, `start_timestamp`, `end_timestamp`, `sort`, and `order`. Its owner scope is derived from authentication, never from the body. Keyword search mirrors the existing separate keyword route; applying form filters exits keyword mode. `POST /api/log/delete_selected` accepts only an explicit UUID selection, targets `LOG_DB`, and never deletes provisional records.

## Bounds and failure semantics

Selections are bounded to 10,000 records; log resolution additionally has a 30-second query deadline and 16 MiB content budget. Oversized sets fail with a request to narrow the filters, never a silently truncated selection. Network failures can occur after a commit: the UI refreshes after attempted mutations and does not promise rollback for an unknown transport outcome. No automatic retry expands or replays a batch.

## Verification

```sh
cd web/modern
yarn type-check
yarn check:i18n
yarn test run
yarn build:prod

# From the repository root, with the go.mod toolchain available:
go test -race ./model ./controller ./router -run 'Selection|Selected|Reset|ChannelReset' -count=1
go vet ./...
```
