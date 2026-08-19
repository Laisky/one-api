# Modern latest-stable upgrade diagnostics

- Base main SHA: `f414444b1796272231d4e78ea8b2bc1321343226`
- Node: `24.x`
- Yarn: `1.22.22`

## Resolved direct dependency manifest

```json
{
  "name": "one-api-modern",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite --host 0.0.0.0",
    "build": "tsc -b && vite build --emptyOutDir",
    "build:prod": "tsc -b && vite build --mode production --emptyOutDir",
    "preview": "vite preview",
    "lint": "eslint .",
    "lint:fix": "eslint . --fix",
    "check:i18n": "node scripts/check-i18n-locales.mjs",
    "type-check": "tsc --noEmit",
    "test": "vitest",
    "test:ui": "vitest --ui"
  },
  "dependencies": {
    "@hookform/resolvers": "^5.9.1",
    "@radix-ui/react-checkbox": "^1.3.11",
    "@radix-ui/react-dialog": "^1.1.23",
    "@radix-ui/react-dropdown-menu": "^2.1.24",
    "@radix-ui/react-hover-card": "^1.1.23",
    "@radix-ui/react-label": "^2.1.15",
    "@radix-ui/react-popover": "^1.1.23",
    "@radix-ui/react-scroll-area": "^1.2.18",
    "@radix-ui/react-select": "^2.3.7",
    "@radix-ui/react-separator": "^1.1.15",
    "@radix-ui/react-slot": "^1.3.3",
    "@radix-ui/react-switch": "^1.3.7",
    "@radix-ui/react-tabs": "^1.1.21",
    "@radix-ui/react-toast": "^1.2.23",
    "@radix-ui/react-tooltip": "^1.2.16",
    "@simplewebauthn/browser": "^13.3.0",
    "@tanstack/react-query": "^5.101.4",
    "@tanstack/react-table": "^9.1.2",
    "@types/dompurify": "^3.2.0",
    "axios": "^1.19.0",
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "cmdk": "^1.1.1",
    "dompurify": "^3.4.14",
    "i18next": "^26.3.6",
    "i18next-browser-languagedetector": "^8.2.1",
    "lucide-react": "^1.33.0",
    "marked": "^18.0.10",
    "qrcode": "^1.5.4",
    "react": "^19.2.8",
    "react-dom": "^19.2.8",
    "react-hook-form": "^7.85.0",
    "react-i18next": "^17.0.11",
    "react-markdown": "^10.1.0",
    "react-router-dom": "^7.18.2",
    "recharts": "^3.10.1",
    "rehype-highlight": "^7.0.2",
    "rehype-katex": "^7.0.1",
    "rehype-raw": "^7.0.0",
    "rehype-sanitize": "^6.0.0",
    "remark-emoji": "^5.0.2",
    "remark-gfm": "^4.0.1",
    "remark-math": "^6.0.0",
    "tailwind-merge": "^3.6.0",
    "zod": "^4.4.3",
    "zustand": "^5.0.15"
  },
  "devDependencies": {
    "@eslint/js": "^10.0.1",
    "@tailwindcss/typography": "^0.5.20",
    "@testing-library/jest-dom": "^7.0.1",
    "@testing-library/react": "^16.3.2",
    "@testing-library/user-event": "^14.6.5",
    "@types/node": "^26.2.0",
    "@types/react": "^19.2.18",
    "@types/react-dom": "^19.2.4",
    "@vitejs/plugin-react": "^6.0.5",
    "autoprefixer": "^10.5.4",
    "browserslist": "^4.28.8",
    "caniuse-lite": "^1.0.30001809",
    "eslint": "^10.8.1",
    "eslint-plugin-react-hooks": "^7.1.1",
    "eslint-plugin-react-refresh": "^0.5.4",
    "globals": "^17.11.0",
    "jsdom": "^30.0.1",
    "postcss": "^8.5.26",
    "prettier": "^3.9.6",
    "tailwindcss": "^4.3.3",
    "tailwindcss-animate": "^1.0.7",
    "typescript": "~7.0.2",
    "typescript-eslint": "^8.67.0",
    "vite": "^8.2.1",
    "vitest": "^4.1.11"
  }
}
```

## Check results

```text
upgrade-direct-dependencies=0
install-upgraded-graph=0
verify-direct-dependencies-current=0
type-check=1
lint=2
i18n=0
unit-tests=1
production-build=1
frozen-lockfile-install=0
diff-check=0
```

## lint (failed)

```text
yarn run v1.22.22
$ eslint .

Oops! Something went wrong! :(

ESLint: 10.8.1

ESLint couldn't find an eslint.config.(js|mjs|cjs) file.

From ESLint v9.0.0, the default configuration file is now eslint.config.js.
If you are using a .eslintrc.* file, please follow the migration guide
to update your configuration file to the new format:

https://eslint.org/docs/latest/use/configure/migration-guide

If you still have problems after following the migration guide, please stop by
https://eslint.org/chat/help to chat with the team.

error Command failed with exit code 2.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## production-build (failed)

```text
yarn run v1.22.22
$ tsc -b && vite build --mode production --emptyOutDir
tsconfig.json(13,5): error TS5102: Option 'baseUrl' has been removed. Please remove it from your configuration.
  Use '"paths": {"*": ["./*"]}' instead.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## type-check (failed)

```text
yarn run v1.22.22
$ tsc --noEmit
tsconfig.json(13,5): error TS5102: Option 'baseUrl' has been removed. Please remove it from your configuration.
  Use '"paths": {"*": ["./*"]}' instead.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## unit-tests (failed)

```text
yarn run v1.22.22
$ vitest --run --passWithNoTests

[1m[30m[46m RUN [49m[39m[22m [36mv4.1.11 [39m[90m/home/runner/work/one-api/one-api/web/modern[39m

 [31m❯[39m src/hooks/__tests__/useRealtimeChat.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/users/__tests__/UsersPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/LoginPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/channelSubmitPayload.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/__tests__/LogDetailsModal.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/ChannelsPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/hooks/useChannelForm.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/components/__tests__/ChannelModelSettings.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/PersonalSettings.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/LoginPage.turnstile.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/tokens/__tests__/TokensPage.share.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/lib/__tests__/api.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/chat/__tests__/ParametersPanel.realtime.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/helpers.jsonc.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/layout/Header.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/RegisterPage.turnstile.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/hooks/__tests__/usePersistentState.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/__tests__/SystemSettings.sensitive.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/test/responsive-components.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/topup/__tests__/TopUpPage.amount.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/hooks/__tests__/useEventLog.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/lib/export.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/hooks/__tests__/usePlaygroundChat.events.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/lib/__tests__/utils.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/chat/__tests__/EventLogPanel.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/lib/__tests__/model-capabilities.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/components/__tests__/ChannelSpecificConfig.key-prompts.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/auth/__tests__/ProtectedRoute.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/users/__tests__/EditUserPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/topup/TopUpPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/logs/__tests__/LogsPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/OidcOAuthPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/WeChatOAuthPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/__tests__/SystemSettings.email-whitelist.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/__tests__/SystemSettings.failure.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/ChannelsPage.balance.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/tokens/__tests__/TokensPage.actions.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/hooks/__tests__/useNotice.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/__tests__/SystemSettings.oidc-discovery.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/redemptions/__tests__/RedemptionsPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/ChannelsPage.bulk-status.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/ChannelsPage.priority.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/__tests__/OperationSettings.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/HomePage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/layout/__tests__/NoticeBanner.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/ui/__tests__/searchable-dropdown.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/RegisterPage.oauth.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/shared/__tests__/NameWithId.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/logs/components/__tests__/LogModelCell.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/SystemSettings.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/tokens/__tests__/TokensPage.highlight.spec.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/__tests__/NotFoundPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/auth/__tests__/RegisterPage.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/tokens/__tests__/useClipboardManager.spec.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/dashboard/components/ToolUsageCharts.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/ui/__tests__/timestamp.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/ui/form.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/settings/__tests__/OtherSettings.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/components/layout/__tests__/Header.topup.test.tsx [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/EditChannelPage.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/lib/__tests__/oauth.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/models/tier-threshold.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/channels/__tests__/constants.deepinfra.test.ts [2m([22m[2m0 test[22m[2m)[22m
 [31m❯[39m src/pages/dashboard/DashboardPage.test.ts [2m([22m[2m0 test[22m[2m)[22m

[31m⎯⎯⎯⎯⎯⎯[39m[1m[41m Failed Suites 64 [49m[22m[31m⎯⎯⎯⎯⎯⎯[39m

[41m[1m FAIL [22m[49m src/lib/export.test.ts[2m [ src/lib/export.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/HomePage.test.tsx[2m [ src/pages/HomePage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/test/responsive-components.test.tsx[2m [ src/test/responsive-components.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/__tests__/LogDetailsModal.test.tsx[2m [ src/components/__tests__/LogDetailsModal.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/layout/Header.test.tsx[2m [ src/components/layout/Header.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/ui/form.test.tsx[2m [ src/components/ui/form.test.tsx ][22m
[41m[1m FAIL [22m[49m src/hooks/__tests__/useEventLog.test.ts[2m [ src/hooks/__tests__/useEventLog.test.ts ][22m
[41m[1m FAIL [22m[49m src/hooks/__tests__/useNotice.test.ts[2m [ src/hooks/__tests__/useNotice.test.ts ][22m
[41m[1m FAIL [22m[49m src/hooks/__tests__/usePersistentState.test.ts[2m [ src/hooks/__tests__/usePersistentState.test.ts ][22m
[41m[1m FAIL [22m[49m src/hooks/__tests__/usePlaygroundChat.events.test.ts[2m [ src/hooks/__tests__/usePlaygroundChat.events.test.ts ][22m
[41m[1m FAIL [22m[49m src/hooks/__tests__/useRealtimeChat.test.ts[2m [ src/hooks/__tests__/useRealtimeChat.test.ts ][22m
[41m[1m FAIL [22m[49m src/lib/__tests__/api.test.ts[2m [ src/lib/__tests__/api.test.ts ][22m
[41m[1m FAIL [22m[49m src/lib/__tests__/model-capabilities.test.ts[2m [ src/lib/__tests__/model-capabilities.test.ts ][22m
[41m[1m FAIL [22m[49m src/lib/__tests__/oauth.test.ts[2m [ src/lib/__tests__/oauth.test.ts ][22m
[41m[1m FAIL [22m[49m src/lib/__tests__/utils.test.ts[2m [ src/lib/__tests__/utils.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/__tests__/NotFoundPage.test.tsx[2m [ src/pages/__tests__/NotFoundPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/EditChannelPage.test.ts[2m [ src/pages/channels/EditChannelPage.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/dashboard/DashboardPage.test.ts[2m [ src/pages/dashboard/DashboardPage.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/models/tier-threshold.test.ts[2m [ src/pages/models/tier-threshold.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/settings/PersonalSettings.test.tsx[2m [ src/pages/settings/PersonalSettings.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/SystemSettings.test.tsx[2m [ src/pages/settings/SystemSettings.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/topup/TopUpPage.test.tsx[2m [ src/pages/topup/TopUpPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/auth/__tests__/ProtectedRoute.test.tsx[2m [ src/components/auth/__tests__/ProtectedRoute.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/chat/__tests__/EventLogPanel.test.tsx[2m [ src/components/chat/__tests__/EventLogPanel.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/chat/__tests__/ParametersPanel.realtime.test.tsx[2m [ src/components/chat/__tests__/ParametersPanel.realtime.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/layout/__tests__/Header.topup.test.tsx[2m [ src/components/layout/__tests__/Header.topup.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/layout/__tests__/NoticeBanner.test.tsx[2m [ src/components/layout/__tests__/NoticeBanner.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/shared/__tests__/NameWithId.test.tsx[2m [ src/components/shared/__tests__/NameWithId.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/ui/__tests__/searchable-dropdown.test.tsx[2m [ src/components/ui/__tests__/searchable-dropdown.test.tsx ][22m
[41m[1m FAIL [22m[49m src/components/ui/__tests__/timestamp.test.tsx[2m [ src/components/ui/__tests__/timestamp.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/LoginPage.test.tsx[2m [ src/pages/auth/__tests__/LoginPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/LoginPage.turnstile.test.tsx[2m [ src/pages/auth/__tests__/LoginPage.turnstile.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/OidcOAuthPage.test.tsx[2m [ src/pages/auth/__tests__/OidcOAuthPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/RegisterPage.oauth.test.tsx[2m [ src/pages/auth/__tests__/RegisterPage.oauth.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/RegisterPage.test.tsx[2m [ src/pages/auth/__tests__/RegisterPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/RegisterPage.turnstile.test.tsx[2m [ src/pages/auth/__tests__/RegisterPage.turnstile.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/auth/__tests__/WeChatOAuthPage.test.tsx[2m [ src/pages/auth/__tests__/WeChatOAuthPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/ChannelsPage.balance.test.tsx[2m [ src/pages/channels/__tests__/ChannelsPage.balance.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/ChannelsPage.bulk-status.test.tsx[2m [ src/pages/channels/__tests__/ChannelsPage.bulk-status.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/ChannelsPage.priority.test.tsx[2m [ src/pages/channels/__tests__/ChannelsPage.priority.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/ChannelsPage.test.tsx[2m [ src/pages/channels/__tests__/ChannelsPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/channelSubmitPayload.test.ts[2m [ src/pages/channels/__tests__/channelSubmitPayload.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/constants.deepinfra.test.ts[2m [ src/pages/channels/__tests__/constants.deepinfra.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/channels/__tests__/helpers.jsonc.test.ts[2m [ src/pages/channels/__tests__/helpers.jsonc.test.ts ][22m
[41m[1m FAIL [22m[49m src/pages/channels/hooks/useChannelForm.test.tsx[2m [ src/pages/channels/hooks/useChannelForm.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/dashboard/components/ToolUsageCharts.test.tsx[2m [ src/pages/dashboard/components/ToolUsageCharts.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/logs/__tests__/LogsPage.test.tsx[2m [ src/pages/logs/__tests__/LogsPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/redemptions/__tests__/RedemptionsPage.test.tsx[2m [ src/pages/redemptions/__tests__/RedemptionsPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/__tests__/OperationSettings.test.tsx[2m [ src/pages/settings/__tests__/OperationSettings.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/__tests__/OtherSettings.test.tsx[2m [ src/pages/settings/__tests__/OtherSettings.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/__tests__/SystemSettings.email-whitelist.test.tsx[2m [ src/pages/settings/__tests__/SystemSettings.email-whitelist.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/__tests__/SystemSettings.failure.test.tsx[2m [ src/pages/settings/__tests__/SystemSettings.failure.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/__tests__/SystemSettings.oidc-discovery.test.tsx[2m [ src/pages/settings/__tests__/SystemSettings.oidc-discovery.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/settings/__tests__/SystemSettings.sensitive.test.tsx[2m [ src/pages/settings/__tests__/SystemSettings.sensitive.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/tokens/__tests__/TokensPage.actions.test.tsx[2m [ src/pages/tokens/__tests__/TokensPage.actions.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/tokens/__tests__/TokensPage.highlight.spec.ts[2m [ src/pages/tokens/__tests__/TokensPage.highlight.spec.ts ][22m
[41m[1m FAIL [22m[49m src/pages/tokens/__tests__/TokensPage.share.test.tsx[2m [ src/pages/tokens/__tests__/TokensPage.share.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/tokens/__tests__/useClipboardManager.spec.ts[2m [ src/pages/tokens/__tests__/useClipboardManager.spec.ts ][22m
[41m[1m FAIL [22m[49m src/pages/topup/__tests__/TopUpPage.amount.test.tsx[2m [ src/pages/topup/__tests__/TopUpPage.amount.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/users/__tests__/EditUserPage.test.tsx[2m [ src/pages/users/__tests__/EditUserPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/users/__tests__/UsersPage.test.tsx[2m [ src/pages/users/__tests__/UsersPage.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/components/__tests__/ChannelModelSettings.test.tsx[2m [ src/pages/channels/components/__tests__/ChannelModelSettings.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/channels/components/__tests__/ChannelSpecificConfig.key-prompts.test.tsx[2m [ src/pages/channels/components/__tests__/ChannelSpecificConfig.key-prompts.test.tsx ][22m
[41m[1m FAIL [22m[49m src/pages/logs/components/__tests__/LogModelCell.test.tsx[2m [ src/pages/logs/components/__tests__/LogModelCell.test.tsx ][22m
[31m[1mError[22m: Cannot find package '@testing-library/dom' imported from /home/runner/work/one-api/one-api/web/modern/node_modules/@testing-library/jest-dom/dist/matchers-b01dabb1.mjs[39m
[31m[2m⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[1/64]⎯[22m[39m


[2m Test Files [22m [1m[31m64 failed[39m[22m[90m (64)[39m
[2m      Tests [22m [2mno tests[22m
[2m   Start at [22m 22:29:12
[2m   Duration [22m 22.70s[2m (transform 58ms, setup 0ms, import 0ms, tests 0ms, environment 54.53s)[22m

error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```
