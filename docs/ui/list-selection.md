# Modern list selection

Channels, Users, Tokens, Redemptions, MCP Servers, and Logs share the same desktop/mobile selection controls.

- A row checkbox adds/removes that record by public UUID, never by page-local position.
- **Select this page** adds the visible page. The header checkbox can deselect only that page, preserving other selections.
- **Select all pages** selects every record matching the applied list scope, not unrelated filtered-out records. Unchecked records become explicit exclusions.
- The compact selection trigger shows the count and distinguishes explicit selection from all matching pages; its menu and accessible status retain the complete scope and exclusion count; cursor lists without an exact count say so. The server resolves the exact action targets before confirmation.
- Pagination, sorting, and page-size changes preserve selection. Filter or principal changes clear it and invalidate pending confirmations. Until edited filters are applied, stale displayed rows cannot be selected.
- The controls do not store credentials or whole records in browser persistence.

## Selected actions

Channel toolbar actions are **Test selected channels**, **Reset selected models**, and **Delete selected disabled channels**. They appear inside the contextual **Actions** menu only after a selection. While loading or running a batch, the menu and commands are disabled. Individual row actions remain available. Disabled-channel deletion checks each channel's current status inside its transaction and skips enabled channels.

Log actions are **Export selected logs** and **Delete selected logs**. Export enriches only selected rows with trace data. Deletion requires administrator access. Ordinary users can resolve/export only their own logs, regardless of UUIDs submitted by the browser.

Add, Search, Refresh, sorting, and statistics are view/creation controls, not record mutations; they retain their existing purpose. This change does not introduce new bulk account or token deletion capabilities. Legacy frontend routes remain compatible.

## Compact toolbar

Both table renderers use `TableToolbar`, rather than independently stacking search, batch buttons, and selection controls.

- A single desktop row contains the compact selection menu, flexible search, icon-only Search/Refresh, and contextual **Actions** menu. No disabled bulk-action strip or separate selection bar occupies space in the idle state.
- The selection menu retains **Select this page**, **Select all pages**, and **Clear selection**. The table header checkbox remains the one-click way to select/deselect the visible page. `All N` is deliberately different from `N selected`; the menu explains filtered scope and exclusions. Unknown cursor counts are not fabricated.
- The action menu retains full command labels and existing confirmation, authorization, selected-UUID snapshots, and failure handling. Destructive commands are separated and styled as destructive inside the menu, not promoted to a persistent red primary button.
- Container queries switch narrow table containers to two deliberate rows: search above, selection and action controls below. Mobile commands keep 44-pixel targets instead of shrinking to fit. Search and Refresh retain accessible labels and keyboard/focus tooltips; menu Escape returns focus to the trigger.
- Log export/deletion now live next to their table selection, not in the page title. The redemption search joins the shared toolbar. Users, Tokens, and MCP Servers also receive the shared compact selection controls without gaining unrelated bulk mutations.
- Free-text search remains visible in the search trigger even when it is not one of the suggestion options.

Design references reviewed on 2026-09-24: [Carbon data tables](https://carbondesignsystem.com/components/data-table/usage/) for contextual batch actions and toolbar grouping; [PatternFly bulk selection](https://www.patternfly.org/patterns/bulk-selection/) for page/all/none selection scope; [WCAG target size guidance](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum) for usable targets. These inform the design; this change does not claim a complete application accessibility audit.

## API contract

Resolve a channel selection with `POST /api/channel/selection`:

```json
{ "keyword": "provider-", "selection": { "mode": "all_matching", "excluded_ids": ["018fcf6d-c484-7000-8000-000000000101"] } }
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

### Real-browser layout checks

After the frontend build, the optional browser check renders the actual Channels (`EnhancedDataTable`) and Redemptions (`DataTable`) pages and production CSS in Chromium. All HTTP calls use a synthetic Axios adapter and network requests are blocked; this is **browser UI integration**, not live-backend end-to-end acceptance.

```sh
# Requires Python Playwright and a local Chromium installation in the test environment.
cd web/modern
python scripts/test-table-toolbar-browser.py --browser /usr/bin/chromium
```

It checks 100 combinations: 320/390/640/888/1280-pixel viewports, all five locales, light/dark themes, and both table renderers. Assertions cover overflow, overlapping controls, click-target size, unchanged toolbar height on selection, menu bounds, Escape/focus, and no mutation when opening menus. An additional real-menu confirmation flow verifies cancellation and exactly one explicit-UUID reset after confirmation. Screenshots and measured geometry are written to `test-results/table-toolbar`.

For an equal-fixture baseline at commit `89a6910e`, the Channels toolbar measured 150 CSS pixels at an 888-pixel viewport (790-pixel table container) and 146 at 1280 (1166-pixel table container). The compact layout measures 36 in both desktop cases: 76.0% and 75.3% less toolbar height. Narrow containers deliberately use two rows, not a forced single-line layout.
