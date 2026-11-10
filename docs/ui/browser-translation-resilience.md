# Browser translation and React text ownership

Browser translation can replace a React-owned text node with generated elements.
React can then throw `NotFoundError` while removing that text or inserting a
sibling. Updating a detached text node can also succeed silently while leaving
the visible label stale. The production `/tokens` event is consistent with this
failure class; its original browser state has not been recovered.

## Shared approach

Modern text-bearing controls use `withTextBoundaries` from
`web/modern/src/components/ui/text-boundaries.tsx`. It places nonblank scalar text
inside stable spans and handles arrays and React Fragments. React can remove the
span or update its text content even after translation replaces its contents.
Element children keep their identity, refs, event handlers and layout position;
the helper does not traverse arbitrary caller-owned elements or custom
components. `asChild` still receives its original single element. Whitespace-only
children are preserved without adding flex items.
Leading and trailing whitespace stays outside inline boundaries so adjacent
labels retain their original accessible names as well as their visible spacing.

Radix Select requires protection inside `ItemText`, because Radix portals those
children into `SelectValue`. The placeholder and explicit value children use the
same helper. Rich `Trans` strings use the application-wide
`react.transWrapTextNodes: 'span'` option documented by
[react-i18next](https://react.i18next.com/latest/trans-component).

For native markup or legacy components, give changing text its own element or
combine adjacent text expressions into one scalar child. A wrapper around an
entire mixed icon/text container is insufficient: the changing text must remain
an independently owned text boundary. Test both removals and visible updates.

Text boundaries are a convention for dynamic mixed-content JSX, not a guarantee
that a primitive protects every descendant. `withTextBoundaries` handles only a
primitive's own scalar, array and Fragment children; it deliberately leaves
caller-owned elements untouched so their identity, refs and layout survive.
`<Button><span><Icon />{isLoading ? 'Loading' : 'Save'}</span></Button>` is
therefore still unsafe: the caller owns that `span`, so it must give the label
its own element, as in `<span><Icon /><span>{label}</span></span>`. Values that
can be `0` must not be rendered through `&&`; use a ternary that yields `null`.
Add a focused translated-text test whenever a new text-bearing control or a
state-driven label beside an icon is introduced.

Lint enforcement was evaluated and not adopted. `eslint-plugin-react-google-translate`
1.1.0, the maintained ESLint 9 option, reports only conditional text. On Modern
it produced 30 reports at 19 locations: 17 are direct children of primitives that
`withTextBoundaries` already protects, and the other 2 remount with their data.
It does not report the MCP status pattern fixed below. React has not changed its
position on translator-mutated DOM ([react#11538](https://github.com/facebook/react/issues/11538)),
so the convention is enforced by behavior tests instead.

This follows React's ownership model: application code should avoid altering
nodes React manages. [React DOM guidance](https://react.dev/learn/manipulating-the-dom-with-refs).
The implementation keeps browser translation available and does not replace
`Node.prototype` methods, swallow reconciliation exceptions, or add dependencies.
The upstream [Radix Select report](https://github.com/radix-ui/primitives/issues/2578)
documents the same selected-label and placeholder failure patterns.

## Audit and regression evidence

The audit covered Modern pages, hooks and reusable controls, Air/Berry source,
direct DOM operations, portal roots, mixed text and element siblings, conditional
labels, scalar updates and locale changes. Default delegates to Modern in the
current server configuration; no separate Default source tree is present.

Confirmed cases include:

| Surface | Behavior reproduced before fixing |
| --- | --- |
| Select and token pagination | Opening a translated selected value throws; a translated placeholder breaks selection. |
| Text-bearing Modern controls | Removing text, inserting icons and updating mixed labels can throw or leave stale text. |
| Chat and action controls | Send/Stop, pending and compact label transitions fail after text replacement. |
| Export metadata | Removing a discovered model throws; count/model updates remain stale. |
| Rich translations | Changing the real application locale leaves translated mixed text stale. |
| Native labels and counters | Changing provider, reasoning status, validation status, cursor warning, latency, chart values or counts can leave stale text. |
| Native locale-sensitive text | Authentication prompts, version labels, pricing headers and timestamp prefixes can remain in the previous locale. |
| Air/Berry log sorting | Switching each sortable column removes a translated arrow through a stale node reference. |
| Air migration counts | Zero-to-positive crashes; positive-to-positive leaves a stale count. |
| Channel response time | Testing an untested channel removes a translated bare `-` and throws; untested rows also rendered a stray `0` from `test_time && …`. |
| MCP server status | Refreshing a translated status swaps the icon before a detached label and throws. |

Air's real log component also had an independent initialization failure: column
definitions accessed sort state/helpers before initialization. The retained
component-body test reproduces that failure; column construction is now deferred
until those values are initialized, preserving hook order.

Negative controls exclude ordinary menu/popover portal closure, sole-text host
updates and privately created download/clipboard nodes. Existing Turnstile code
owns its third-party subtree inside an otherwise empty React container. Unused
menu variants and sole-text consumers are not reported as production crashes.

Modern tests share `src/test/dom-translation.tsx`, which replaces actual mounted
text nodes with nested translator-style `font` elements and returns a mutation
count. Regressions assert nonempty mutation, real state/event transitions, visible
content and absence of captured exceptions. Compatibility cases retain keyboard
selection, form values, refs, Slot events, keyed component identity and user input.
The login compatibility test checks the complete branded heading's visible text
and accessible name; a missing separating space between inline boundaries is
also covered by a dedicated regression.
Legacy tests execute actual source JSX with React 18; their source selection
supports historical negative controls through `LEGACY_UI_BASELINE`.

## Request state captured by event handlers

Review of the legacy log sorting fix found a second, unrelated class: a handler
calls a state setter and then immediately builds a request from the same state,
which still holds the previous render's value
([React: state as a snapshot](https://react.dev/learn/state-as-a-snapshot)).
The standard applied everywhere is the one Modern's log page already used:

- A handler computes the next query (sort, type, range, page size) and passes it
  to the loader explicitly; loaders default only to committed state.
- Exactly one code path owns each reload, so one user action sends one request.
- The log row loaders and Air dashboard quota loader commit only the current
  request's results. Modern's keyset hook accepts
  `fetchPage(navigation, { pageSize, isCurrent })` and ignores a stale response
  instead of moving the cursor under newer rows. Air/Berry use one
  `createLatestRequest` gate per mounted resource, shared across the listing and
  search endpoints for the same table. A new request owns rows, search mode,
  counts, component errors and loading cleanup. Berry commits uncached page
  navigation inside that boundary, preserving its previous page while loading.
  Handlers that select a page immediately never repeat navigation after awaiting
  the request, so an old completion cannot move the pager. Selecting a cached
  Berry page explicitly invalidates pending navigation even though it sends no
  new request. Paging is temporarily disabled during a new Air/Berry search, refresh
  or sort because cached pages still belong to the previous query; page fetches
  continue to allow navigation back to an already cached page.
- Unmounting deactivates the gate. Effect setup reactivates it without restoring
  superseded requests; this preserves React StrictMode's setup/cleanup replay,
  including the dashboard's already-initialized chart request.

This is not an application-wide cancellation mechanism. Requests are not aborted,
and API interceptors retain their existing global transport-error reporting and
authentication behavior. The gate suppresses obsolete component-owned results
and errors only. Independent statistics, autocomplete and tracing requests are
not covered by this guarantee or allowed to supersede table/chart requests.

| Surface | Behavior reproduced before fixing |
| --- | --- |
| Air/Berry log sorting | The first sort sent no sort, toggles sent the previous direction, and keyword-search sorting sent the previous column. |
| Air log type filter | Paging and page-size changes sent `type=0`; Query sent the click event (`type=[object Object]`). |
| Berry log refresh | Mount and refresh sent two requests; the first still carried the cleared keyword. Timestamps were fractional, which the server parses as `0`. |
| Air dashboard presets | Today/7/30-day buttons requested the previous range. |
| Air/Berry concurrent log requests | An older listing/search could replace newer rows, restore an obsolete endpoint mode/count, clear current busy state, or report an obsolete component error. Berry late page requests could also move the pager after a refresh. |
| Air concurrent dashboard ranges | An older range could redraw both charts after a newer range or after unmount, and clear current loading. |
| Air LobeChat link | The token menu opened LobeChat with `sk-undefined`. |
| Modern log page size | Cursor mode sent a stale-size request whose late response moved Next to the wrong cursor; offset mode sent three requests. |

The legacy suites also resolve every compound UI-library member the source
reads, such as `Form.Slot` or `Select.Option`, against the installed Semi UI or
MUI packages. Bundlers check only named imports, so an undefined member fails at
render time. This found Air's admin log filter (`Form.Field`, which Semi does not
define, so the admin log page crashed) and Air's models page (`Collapsible.Panel`,
so every channel card rendered empty).

Legacy behavior tests mount the real page modules with the theme's own React and
UI library through `.github/scripts/legacy-react-harness.cjs`. The suites replace
transport, toast notifications, authentication predicates, copy behavior, selected
timestamp/number/quota/color formatting helpers, unrelated modals, and reference
tooltip wrappers. Air dashboard tests also replace `@visactor/vchart` with
`FakeChart`. Assertions read actual request parameters and rendered page text;
the deferred dashboard tests inspect the specifications delivered to both chart
instances. These doubles do not qualify authentication integration, the replaced
formatting/copy/tooltip helpers, or real chart/canvas rendering. The jsdom harness
also supplies inert layout, media-query and canvas APIs.

The concurrency regressions settle real page requests in deliberately reversed
orders. They cover admin/self routes, listing/search transitions, pagination,
success and both application/transport failures, loading, unmount and effect
replay. Current errors remain visible and existing request-count, sort, filter
and timestamp assertions remain negative controls. Rerun against the prior PR
head to reproduce the confirmed races without changing the regression code:

```sh
(cd web/air && LEGACY_UI_BASELINE=abbabd17 node --test ../../.github/scripts/legacy-log-requests.test.cjs ../../.github/scripts/legacy-air-pages.test.cjs)
(cd web/berry && LEGACY_UI_BASELINE=abbabd17 node --test ../../.github/scripts/legacy-log-requests.test.cjs)
```

No endpoint, query-parameter dialect, role routing, stored data, dependency, or
normal sequential-request behavior changes. Previously issued requests still
finish; only the latest operation owns component state. A current failed request
keeps the prior successful rows while reporting its error.

Run the complete frontend suites and builds from the repository root:

```sh
yarn --cwd web/modern test --run --maxWorkers=4
yarn --cwd web/modern lint
make build-frontend-modern
yarn --cwd web/air test
make build-frontend-air
yarn --cwd web/berry test
make build-frontend-berry
```

### Latest-request follow-up validation

On the follow-up to `abbabd17`, the same regression source against that revision
fails 17 Air cases (including 5 dashboard cases) and 14 Berry cases. Passing
negative controls include sequential filters/sorts, current errors and effect
replay. These are failing behavioral cases, not 31 distinct defects.

- Air full frontend suite: 209 passed, 2 existing React19-only skips.
- Berry full frontend suite: 186 passed, 2 existing React19-only skips.
- Air and Berry lint and production builds passed. Builds run sequentially;
  running both builds concurrently in the cloud executor was killed with exit
  137. Existing dependency/chunk-size and Berry lint warnings remain.
- Release-input guard: 7 tests passed; frontend CI policy contracts: 4 passed.
  The new shared helper is explicitly declared for both importing themes.
- `make test-quick` passed all 28 selected tests and `go vet ./...` passed with
  Go 1.27.3 after using the available toolchain. Full Go race/database qualification, the unchanged Modern
  frontend and real-browser/production qualification were not rerun here.
  Exact-commit CI remains a separate publication check.

These tests simulate the relevant DOM mutation; they do not qualify every browser
translator or extension. Extensions that replace entire owned elements, arbitrary
third-party internals and future caller-owned mixed markup need separate behavior
tests. Production rollout and post-release error monitoring remain separate from
local qualification. No API, stored data, export payload or migration format is
changed by these text-boundary fixes.
