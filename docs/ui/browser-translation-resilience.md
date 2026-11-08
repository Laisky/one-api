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

These tests simulate the relevant DOM mutation; they do not qualify every browser
translator or extension. Extensions that replace entire owned elements, arbitrary
third-party internals and future caller-owned mixed markup need separate behavior
tests. Production rollout and post-release error monitoring remain separate from
local qualification. No API, stored data, export payload or migration format is
changed by these text-boundary fixes.
