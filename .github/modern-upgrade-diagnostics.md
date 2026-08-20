# Modern latest-stable compatibility diagnostics

- Resolver base: latest `main` at staging reset
- Node: `24.x`
- Yarn: `1.22.22`

## Check status

```text
current=0
type-check=1
lint=0
i18n=0
unit-tests=1
production-build=1
frozen-lockfile=0
```

## Vitest summary

- suites: 152 total, 36 failed
- tests: 351 total, 48 failed

```text
Enhanced Data Table > should handle responsive props
  TypeError: row.getIsSelected is not a function
      at /Users/runner/work/one-api/one-api/web/modern/src/components/ui/enhanced-data-table.tsx:390:41
      at Array.map (<anonymous>)
      at EnhancedDataTable (/Users/runner/work/one-api/one-api/web/modern/src/components/ui/enhanced-data-table.tsx:387:46)
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25904:20)
      at renderWithHooks (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:7662:22)
      at updateFunctionComponent (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:10166:19)
      at beginWork (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:11778:18)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at performUnitOfWork (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:17641:22)
      at workLoopSync (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:17469:41)
Enhanced Data Table > should hide floating row actions while hovering interactive row controls
  TypeError: row.getIsSelected is not a function
      at /Users/runner/work/one-api/one-api/web/modern/src/components/ui/enhanced-data-table.tsx:390:41
      at Array.map (<anonymous>)
      at EnhancedDataTable (/Users/runner/work/one-api/one-api/web/modern/src/components/ui/enhanced-data-table.tsx:387:46)
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25904:20)
      at renderWithHooks (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:7662:22)
      at updateFunctionComponent (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:10166:19)
      at beginWork (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:11778:18)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at performUnitOfWork (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:17641:22)
      at workLoopSync (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:17469:41)
Header logout UX > hides the logout action by default
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at /Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx:82:22
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
Header logout UX > confirms logout through the desktop hamburger menu
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at /Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx:82:22
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
Header mobile overflow prevention > renders header with no horizontal overflow and truncates brand text
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at /Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx:82:22
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
NameWithId > shows the id when the name is clicked
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at autoUpdate (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@floating-ui/dom/dist/floating-ui.dom.mjs:613:22)
      at Object.whileElementsMounted [as current] (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@radix-ui/react-popper/dist/index.mjs:131:25)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@floating-ui/react-dom/dist/floating-ui.react-dom.mjs:189:40
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookLayoutEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13213:11)
NameWithId > does not bubble name clicks into parent row actions
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at autoUpdate (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@floating-ui/dom/dist/floating-ui.dom.mjs:613:22)
      at Object.whileElementsMounted [as current] (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@radix-ui/react-popper/dist/index.mjs:131:25)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@floating-ui/react-dom/dist/floating-ui.react-dom.mjs:189:40
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookLayoutEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13213:11)
SearchableDropdown > keeps server-provided results when the parent owns the search
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/cmdk/dist/index.mjs:1:8384
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
SearchableDropdown > filters locally when the caller owns the options
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/cmdk/dist/index.mjs:1:8384
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
SearchableDropdown > matches local options on their extra keywords
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/cmdk/dist/index.mjs:1:8384
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
TimestampDisplay > shows the UTC timestamp inside the tooltip
  Error: Unable to find an element with the text: 2024-10-13 18:40:00Z. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/components/ui/__tests__/timestamp.test.tsx:26:42
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage balance refresh > refreshes a single row balance via per-row icon
  Error: Unable to find a label with the text of: Refresh balance for Channel 3
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.balance.test.tsx:82:37
      at runNextTicks (node:internal/process/task_queues:64:5)
      at processTimers (node:internal/timers:518:9)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage balance refresh > refreshes all balances from header bulk action
  Error: Unable to find role="button" and name `/refresh all balances/i`
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.balance.test.tsx:95:34
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage bulk enable/disable > disables all visible channels via bulk action
  Error: Unable to find role="button" and name `/bulk actions/i`
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.bulk-status.test.tsx:62:34
      at runNextTicks (node:internal/process/task_queues:64:5)
      at processTimers (node:internal/timers:518:9)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage priority editor > saves a changed priority on blur
  Error: Unable to find a label with the text of: Priority for Channel 7
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.priority.test.tsx:71:32
      at runNextTicks (node:internal/process/task_queues:64:5)
      at processTimers (node:internal/timers:518:9)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage priority editor > does not call api.put when value is unchanged
  Error: Unable to find a label with the text of: Priority for Channel 7
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.priority.test.tsx:92:32
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage Pagination > should not make duplicate API calls when changing page size
  TestingLibraryElementError: Unable to find an accessible element with the role "combobox" and name `/rows per page/i`
  There are no accessible roles. But there might be some inaccessible roles. If you wish to access them, then set the `hidden` option to `true`. Learn more about this here: https://testing-library.com/docs/dom-testing-library/api-queries#byrole
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at Object.getElementError (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/config.js:37:19)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:76:38
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:52:17
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:95:19
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:110:35
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage Pagination > should handle page navigation correctly
  TestingLibraryElementError: Unable to find an accessible element with the role "button" and name "Page 2"
  There are no accessible roles. But there might be some inaccessible roles. If you wish to access them, then set the `hidden` option to `true`. Learn more about this here: https://testing-library.com/docs/dom-testing-library/api-queries#byrole
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at Object.getElementError (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/config.js:37:19)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:76:38
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:52:17
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:95:19
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:138:32
      at runNextTicks (node:internal/process/task_queues:64:5)
ChannelsPage Pagination > should handle sorting without duplicate calls
  TestingLibraryElementError: Unable to find an accessible element with the role "button" and name `/name/i`
  There are no accessible roles. But there might be some inaccessible roles. If you wish to access them, then set the `hidden` option to `true`. Learn more about this here: https://testing-library.com/docs/dom-testing-library/api-queries#byrole
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at Object.getElementError (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/config.js:37:19)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:76:38
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:52:17
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:95:19
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:161:31
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage Pagination > should duplicate a channel with copied configuration
  Error: Unable to find role="button" and name "Duplicate"
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:183:43
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
ChannelsPage Pagination > should show the channel name and type in the delete confirmation dialog
  Error: Unable to find an element with the text: Channel 1. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:201:35
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
ChannelsPage Pagination > shows an error notification when delete returns success false
  Error: Unable to find an element with the text: Channel 1. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:221:35
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
ChannelsPage Pagination > shows an error notification when bulk test returns success false
  Error: Unable to find an element with the text: Channel 1. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:251:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
ChannelsPage Pagination > only offers text-compatible testing models and clears to CHEAPEST
  Error: Unable to find an element with the text: Filtered Channel. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:291:35
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
ChannelsPage Pagination > filters non-text testing models when the server field is missing
  Error: Unable to find an element with the text: Legacy Channel. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/channels/__tests__/ChannelsPage.test.tsx:335:35
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
ToolUsageCharts > renders the tool dashboard sections
  TypeError: () => ({
  	observe: __vite_ssr_import_0__.vi.fn(),
  	unobserve: __vite_ssr_import_0__.vi.fn(),
  	disconnect: __vit...<omitted>...}) is not a constructor
      at new Mock (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/spy/dist/index.js:309:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/recharts/lib/component/ResponsiveContainer.js:127:20
      at Object.react_stack_bottom_frame (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:25989:20)
      at runWithFiberInDEV (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:874:13)
      at commitHookEffectListMount (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13249:29)
      at commitHookPassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:13336:11)
      at commitPassiveMountOnFiber (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15484:13)
      at recursivelyTraversePassiveMountEffects (/Users/runner/work/one-api/one-api/web/modern/node_modules/react-dom/cjs/react-dom-client.development.js:15439:11)
LogsPage action feedback > shows channel names and reveals channel UUIDs from the channel column
  Error: Unable to find role="button" and name "OpenAI Primary"
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/logs/__tests__/LogsPage.test.tsx:113:40
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
RedemptionsPage action feedback > shows an error when delete returns success false
  Error: Unable to find an element with the text: Redeem A. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/redemptions/__tests__/RedemptionsPage.test.tsx:67:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
RedemptionsPage action feedback > shows an error when status update returns success false
  Error: Unable to find an element with the text: Redeem A. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/redemptions/__tests__/RedemptionsPage.test.tsx:81:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
TokensPage action feedback > shows an error when delete returns success false
  Error: Unable to find an element with the text: Token A. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/__tests__/TokensPage.actions.test.tsx:68:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
TokensPage action feedback > shows an error when status update returns success false
  Error: Unable to find an element with the text: Token A. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/__tests__/TokensPage.actions.test.tsx:81:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
TokensPage share dropdown integration > opens ChatGPT Next Web URL when the dropdown item is selected
  Error: Unable to find an element with the text: Sample. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at Proxy.waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/__tests__/TokensPage.share.test.tsx:140:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
      at runWithCancel (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2323:10)
TokensPage share dropdown integration > hides ChatGPT Next Web and LobeChat when chat_link is empty
  Error: Unable to find an element with the text: Sample. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at Proxy.waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/__tests__/TokensPage.share.test.tsx:168:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
      at runWithCancel (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2323:10)
TokensPage share dropdown integration > shows token name and id in the delete confirmation dialog
  Error: Unable to find an element with the text: Sample. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at Proxy.waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/__tests__/TokensPage.share.test.tsx:185:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
      at runWithCancel (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2323:10)
TopUpPage: Stripe checkout behavior > submits the exact two-decimal USD amount
  Error: Found a label with the text of: /amount \(usd\)/i, however no form control was found associated to that label. Make sure you're using the "for" attribute or "aria-labelledby" attribute correctly.
  Ignored nodes: comments, script, style
  <body>
    <div>
      <div
        class="w-full mx-auto px-8 max-w-4xl"
      >
        <div
          class="mb-4 md:mb-6 space-y-2"
        >
          <div
            class="flex justify-between gap-4 flex-row items-center"
TopUpPage: Stripe checkout behavior > blocks amounts below the server-advertised minimum
  Error: Found a label with the text of: /amount \(usd\)/i, however no form control was found associated to that label. Make sure you're using the "for" attribute or "aria-labelledby" attribute correctly.
  Ignored nodes: comments, script, style
  <body>
    <div>
      <div
        class="w-full mx-auto px-8 max-w-4xl"
      >
        <div
          class="mb-4 md:mb-6 space-y-2"
        >
          <div
            class="flex justify-between gap-4 flex-row items-center"
EditUserPage 2FA disable button > renders disable 2FA button when admin views another user and triggers API call on confirm
  Error: Unable to find role="button" and name `/disable user 2fa/i`
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/EditUserPage.test.tsx:102:33
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > hides Promote action when current user is not super admin
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:132:18
      at runNextTicks (node:internal/process/task_queues:64:5)
      at processTimers (node:internal/timers:518:9)
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
UsersPage promote/demote/disable_2fa actions > shows Promote when super admin views a regular user
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:145:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:20
UsersPage promote/demote/disable_2fa actions > promotes a user, updates the row role, and notifies success
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:161:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > shows error notification when promote fails
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:198:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > demotes an admin and updates the row role
  Error: Unable to find an element with the text: bob. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:218:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > shows error notification when demote fails
  Error: Unable to find an element with the text: bob. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:249:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > deletes a user row when the backend confirms success
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:269:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > shows error notification when delete returns success false
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:290:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > shows an error notification when top up returns success false
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:315:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > disables 2FA after confirmation and notifies success
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:340:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
UsersPage promote/demote/disable_2fa actions > shows error notification when disable 2FA fails
  Error: Unable to find an element with the text: alice. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
  Ignored nodes: comments, script, style
  <body>
    <div />
  </body>
      at waitForWrapper (/Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/wait-for.js:163:27)
      at /Users/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/dom/dist/query-helpers.js:86:33
      at /Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/UsersPage.test.tsx:363:18
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:302:11
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:1903:26
      at file:///Users/runner/work/one-api/one-api/web/modern/node_modules/@vitest/runner/dist/chunk-artifact.js:2326:20
      at new Promise (<anonymous>)
```

## production-build

```text
yarn run v1.22.22
$ tsc -b && vite build --mode production --emptyOutDir
src/components/chat/ChatInterface.tsx(269,13): error TS2322: Type '((messageIndex: number, newContent: string) => void) | undefined' is not assignable to type '((messageIndex: number, newContent: string | any[]) => void) | undefined'.
  Type '(messageIndex: number, newContent: string) => void' is not assignable to type '(messageIndex: number, newContent: string | any[]) => void'.
    Types of parameters 'newContent' and 'newContent' are incompatible.
      Type 'string | any[]' is not assignable to type 'string'.
        Type 'any[]' is not assignable to type 'string'.
src/components/ui/data-table.tsx(95,5): error TS2322: Type 'ModernColumnDef<TData, TValue>[]' is not assignable to type 'readonly ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>[]'.
  Type 'ModernColumnDef<TData, TValue>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
    Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
      Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown> & ColumnDef_RowSorting<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData> & { ...; } & Partial<...>'.
        Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
          Types of property 'footer' are incompatible.
            Type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>> | undefined' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
              Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
                Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>) => any'.
                  Types of parameters 'props' and 'props' are incompatible.
                    Type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                      Types of property 'column' are incompatible.
                        Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                          Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                            Types of property 'accessorFn' are incompatible.
                              Type 'AccessorFn<TData, unknown> | undefined' is not assignable to type 'AccessorFn<TData, TValue> | undefined'.
                                Type 'AccessorFn<TData, unknown>' is not assignable to type 'AccessorFn<TData, TValue>'.
                                  Type 'unknown' is not assignable to type 'TValue'.
                                    'TValue' could be instantiated with an arbitrary type which could be unrelated to 'unknown'.
src/components/ui/data-table.tsx(123,24): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/data-table.tsx(123,47): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/data-table.tsx(162,60): error TS2339: Property 'getIsSelected' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/data-table.tsx(163,28): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/data-table.tsx(163,51): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/enhanced-data-table.tsx(218,5): error TS2322: Type 'ModernColumnDef<TData, TValue>[]' is not assignable to type 'readonly ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>[]'.
  Type 'ModernColumnDef<TData, TValue>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
    Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
      Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown> & ColumnDef_RowSorting<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData> & { ...; } & Partial<...>'.
        Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
          Types of property 'footer' are incompatible.
            Type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>> | undefined' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
              Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
                Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>) => any'.
                  Types of parameters 'props' and 'props' are incompatible.
                    Type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                      Types of property 'column' are incompatible.
                        Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                          Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                            Types of property 'accessorFn' are incompatible.
                              Type 'AccessorFn<TData, unknown> | undefined' is not assignable to type 'AccessorFn<TData, TValue> | undefined'.
                                Type 'AccessorFn<TData, unknown>' is not assignable to type 'AccessorFn<TData, TValue>'.
                                  Type 'unknown' is not assignable to type 'TValue'.
                                    'TValue' could be instantiated with an arbitrary type which could be unrelated to 'unknown'.
src/components/ui/enhanced-data-table.tsx(333,24): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/enhanced-data-table.tsx(333,47): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/enhanced-data-table.tsx(390,41): error TS2339: Property 'getIsSelected' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/enhanced-data-table.tsx(397,30): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/enhanced-data-table.tsx(397,53): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/pages/channels/schemas.ts(59,22): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Type '{}' is missing the following properties from type '{ region?: string | undefined; ak?: string | undefined; sk?: string | undefined; user_id?: string | undefined; vertex_ai_project_id?: string | undefined; vertex_ai_adc?: string | undefined; ... 11 more ...; tencent_secret_key?: string | undefined; }': auth_type, api_format
src/pages/dashboard/components/UsageSections.tsx(2,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/pages/dashboard/components/UsageSections.tsx(3,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/pages/dashboard/components/UsageSections.tsx(4,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/pages/dashboard/components/UsageSections.tsx(5,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/test/setup.ts(105,3): error TS2578: Unused '@ts-expect-error' directive.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## type-check

```text
yarn run v1.22.22
$ tsc --noEmit
src/components/chat/ChatInterface.tsx(269,13): error TS2322: Type '((messageIndex: number, newContent: string) => void) | undefined' is not assignable to type '((messageIndex: number, newContent: string | any[]) => void) | undefined'.
  Type '(messageIndex: number, newContent: string) => void' is not assignable to type '(messageIndex: number, newContent: string | any[]) => void'.
    Types of parameters 'newContent' and 'newContent' are incompatible.
      Type 'string | any[]' is not assignable to type 'string'.
        Type 'any[]' is not assignable to type 'string'.
src/components/ui/data-table.tsx(95,5): error TS2322: Type 'ModernColumnDef<TData, TValue>[]' is not assignable to type 'readonly ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>[]'.
  Type 'ModernColumnDef<TData, TValue>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
    Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
      Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown> & ColumnDef_RowSorting<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData> & { ...; } & Partial<...>'.
        Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
          Types of property 'footer' are incompatible.
            Type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>> | undefined' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
              Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
                Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>) => any'.
                  Types of parameters 'props' and 'props' are incompatible.
                    Type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                      Types of property 'column' are incompatible.
                        Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                          Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                            Types of property 'accessorFn' are incompatible.
                              Type 'AccessorFn<TData, unknown> | undefined' is not assignable to type 'AccessorFn<TData, TValue> | undefined'.
                                Type 'AccessorFn<TData, unknown>' is not assignable to type 'AccessorFn<TData, TValue>'.
                                  Type 'unknown' is not assignable to type 'TValue'.
                                    'TValue' could be instantiated with an arbitrary type which could be unrelated to 'unknown'.
src/components/ui/data-table.tsx(123,24): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/data-table.tsx(123,47): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/data-table.tsx(162,60): error TS2339: Property 'getIsSelected' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/data-table.tsx(163,28): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/data-table.tsx(163,51): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/enhanced-data-table.tsx(218,5): error TS2322: Type 'ModernColumnDef<TData, TValue>[]' is not assignable to type 'readonly ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>[]'.
  Type 'ModernColumnDef<TData, TValue>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
    Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDef<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
      Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown> & ColumnDef_RowSorting<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData> & { ...; } & Partial<...>'.
        Type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue> & ColumnDef_RowSorting<...> & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>'.
          Types of property 'footer' are incompatible.
            Type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>> | undefined' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
              Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type 'ColumnDefTemplate<HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>> | undefined'.
                Type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>) => any' is not assignable to type '(props: HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>) => any'.
                  Types of parameters 'props' and 'props' are incompatible.
                    Type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'HeaderContext<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                      Types of property 'column' are incompatible.
                        Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                          Type 'Column<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, unknown>' is not assignable to type 'Column_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData, TValue>'.
                            Types of property 'accessorFn' are incompatible.
                              Type 'AccessorFn<TData, unknown> | undefined' is not assignable to type 'AccessorFn<TData, TValue> | undefined'.
                                Type 'AccessorFn<TData, unknown>' is not assignable to type 'AccessorFn<TData, TValue>'.
                                  Type 'unknown' is not assignable to type 'TValue'.
                                    'TValue' could be instantiated with an arbitrary type which could be unrelated to 'unknown'.
src/components/ui/enhanced-data-table.tsx(333,24): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/enhanced-data-table.tsx(333,47): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/enhanced-data-table.tsx(390,41): error TS2339: Property 'getIsSelected' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/enhanced-data-table.tsx(397,30): error TS2339: Property 'getVisibleCells' does not exist on type 'Row_Core<{ rowPaginationFeature: TableFeature; rowSortingFeature: TableFeature; }, TData>'.
src/components/ui/enhanced-data-table.tsx(397,53): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/pages/channels/schemas.ts(59,22): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Type '{}' is missing the following properties from type '{ region?: string | undefined; ak?: string | undefined; sk?: string | undefined; user_id?: string | undefined; vertex_ai_project_id?: string | undefined; vertex_ai_adc?: string | undefined; ... 11 more ...; tencent_secret_key?: string | undefined; }': auth_type, api_format
src/pages/dashboard/components/UsageSections.tsx(2,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/pages/dashboard/components/UsageSections.tsx(3,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/pages/dashboard/components/UsageSections.tsx(4,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/pages/dashboard/components/UsageSections.tsx(5,15): error TS2300: Duplicate identifier 'ComponentProps'.
src/test/setup.ts(105,3): error TS2578: Unused '@ts-expect-error' directive.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## unit-tests

```text
yarn run v1.22.22
$ vitest --run --passWithNoTests --reporter=json --outputFile=/tmp/modern-upgrade-logs/vitest.json
JSON report written to /tmp/modern-upgrade-logs/vitest.json
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```
