# Modern Frontend File-Size Strategy

The Modern template now follows a strict upper bound of **800 lines per source file** (ideally ≤600) to keep code reviewable and modular. This document captures the audit performed on 2025-11-23 and the restructuring strategy that keeps the rule enforceable going forward.

## Current Audit Snapshot

| Path                                               | Lines |
| -------------------------------------------------- | ----: |
| `web/modern/src/pages/chat/PlaygroundPage.tsx`     |  1322 |
| `web/modern/src/pages/dashboard/DashboardPage.tsx` |  1212 |

All other Modern source files are under the threshold. The items above are being split immediately as outlined below.

## Layered Page Architecture

Every Modern page should live inside a **feature folder** with the following optional pieces:

```text
/pages/<feature>/
  Page.tsx                // thin routing entry
  hooks/
    useViewModel.ts       // composes domain hooks for the page
    useData.ts            // data fetching + side effects (per concern)
  components/
    SectionA.tsx          // purely presentational blocks
    SectionB.tsx
  services/
    conversions.ts        // feature-specific helpers (no React)
```

Principles:

1. **Page component ≤250 lines.** It should import hooks and render layout only.
2. **View-model hooks ≤400 lines.** Complex logic moves into focused hooks/services. If any hook approaches 400 lines, split further (e.g., `useFilters`, `usePersistence`).
3. **Presentational components ≤200 lines.** Keep them stateless when possible; extract shared UI into `/components/`.
4. **No anonymous utils in pages.** Helpers must live under `services/` or `/lib` to stay testable and reusable.

## Immediate Refactors

### Playground (chat)

New structure under `web/modern/src/pages/chat/playground/`:

- `PlaygroundPage.tsx` (existing path) now imports from the feature folder.
- `hooks/usePlaygroundViewModel.ts` orchestrates business logic.
- `hooks/useConversationPersistence.ts` isolates localStorage lifecycle for conversations.
- `hooks/usePlaygroundParameters.ts` owns parameter defaults, capability resets, and storage.
- `hooks/useModelAndTokenBrowser.ts` loads tokens, channels, models, and suggestion logic.
- `services/codeBlockStyles.ts` injects highlight styles exactly once on the client.

Handlers such as copy/edit/delete live near their relevant hook. The page component simply wires `ParametersPanel`, `ChatInterface`, and `ExportConversationDialog` with the data returned by the view-model hook.

### Dashboard

New structure under `web/modern/src/pages/dashboard/`:

- `DashboardPage.tsx` renders layout + orchestrates.
- `hooks/useDashboardFilters.ts` manages date-range logic, presets, validation, and admin/user filtering.
- `hooks/useDashboardData.ts` handles API calls, abort controllers, and normalization of API payloads.
- `components/FiltersPanel.tsx`, `components/MetricCards.tsx`, `components/TrendCharts.tsx`, `components/TopEntitiesTable.tsx` break the UI into self-contained blocks.
- `services/chartConfig.ts` hosts gradient + color metadata shared by chart components.

Each component consumes typed props derived from the hooks so testing and reuse stay trivial.

## Enforcement & Tooling

- Add `yarn lint:file-sizes` (future work) to fail CI when any file exceeds 800 lines by reusing the Python audit from this doc.
- During code review, require new pages to follow the folder pattern. When an existing file exceeds 600 lines, open a refactor task before merging new features.
- Keep this document updated when introducing new feature folders or shared hooks.
