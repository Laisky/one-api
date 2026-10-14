# Modern table sorting and range selection

## Behavior

Channels, Users, Tokens, Redemptions, MCP Servers, and Logs remember their last selected sort column and direction. Header buttons and existing toolbar/mobile sort controls update the same state. Preferences are scoped to the list and authenticated account/role in this browser; they are not synchronized across devices. Only column/direction metadata is stored, never rows or API keys.

`useTableSort` reads the saved pair before the first list request. Defaults are not written on mount. Malformed, oversized, unsupported-column, or unavailable storage falls back to the existing default sort. A storage write failure preserves working in-memory sorting. The Logs page retains its existing cursor eligibility: non-default sorting uses the offset API.

Click a record checkbox, then Shift-click another checkbox to select the inclusive displayed range. The last ordinary click is the anchor. Reverse ranges work. Shift-clicking an already selected target deselects that range. Selections outside the range remain unchanged, including existing selections on other pages. In all-matching mode, range changes update exclusions atomically rather than converting the selection into a page-only list.

A range never fetches or includes unseen pages. Paging, page-size, sort, filter, account, loading/disabled, and explicit selection-control changes invalidate the range anchor. Existing stable-ID selection semantics remain unchanged. Changing view metadata invalidates the anchor even when the returned row IDs happen to be identical. A Shift-click without a valid visible anchor behaves as a single checkbox activation. Checkbox interaction does not open the record.

## Maintenance

- Keep the supported sort-column registry in `web/modern/src/lib/table-sort-preference.ts` aligned with each page's header and toolbar choices. Page-level tests check both sources of choices. Preserve hidden default IDs and toolbar-only fields.
- New list pages should use `useTableSort` at the request-state owner, not restore preferences in a child table effect after a default request has already started.
- Both shared table renderers pass page/size/sort metadata as the internal range context. Do not include these fields in the selection universe itself: doing so would discard existing cross-page selections.
- New first-party files under Modern `src` or `public`, including tests, must be enumerated in `web/legacy/browser-release-inputs.json`. Do not disable the build-input guard.

## Qualification

Use the repository's supported Node version and locked Yarn dependencies. Fast iteration:

```sh
cd web/modern
yarn install --frozen-lockfile
yarn vitest run --maxWorkers=2 \
  src/components/ui/__tests__/table-selection.test.tsx \
  src/components/ui/__tests__/table-selection-range.test.tsx \
  src/components/ui/__tests__/table-sort-persistence.test.tsx \
  src/hooks/__tests__/useTableSort.test.tsx \
  src/lib/__tests__/table-selection-range.test.ts \
  src/lib/__tests__/table-sort-preference.test.ts \
  src/pages/__tests__/list-sort-persistence.test.tsx
```

Final frontend qualification:

```sh
cd web/modern
yarn vitest run --maxWorkers=2
yarn lint
yarn check:i18n
cd ../..
make build-frontend-modern
python3 -m pip install playwright==1.57.0
python3 -m playwright install chromium
python3 web/modern/scripts/test-table-interactions-browser.py
```

The browser regression runs the actual compiled application at a loopback origin, with every API intercepted using synthetic accounts and records. It verifies first-request sorting, real reloads, header/mobile indicators, visible order, forward/reverse ranges, all-matching exclusions, stale anchors, account isolation, and removed-column fallback on all six pages at desktop and mobile widths. It refuses external requests and API writes. It does not qualify a production backend or deployment. Browser results and screenshots are retained under `web/modern/test-results/table-interactions`.

## Release and rollback

Release through the existing frontend/image delivery path after review and frontend acceptance. There is no database migration, billing change, new runtime dependency, or new permission requirement. Revert this feature and redeploy the previously qualified frontend to roll back; old clients ignore the new `oneapi_table_sort_v1:` storage keys. Do not clear unrelated browser storage or change deployment/security policy.
