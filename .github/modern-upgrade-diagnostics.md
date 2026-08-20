# Modern latest-stable compatibility diagnostics

- Resolver base: latest `main` at staging reset
- Node: `24.x`
- Yarn: `1.22.22`

## Check status

```text
current=0
type-check=127
lint=1
i18n=0
unit-tests=1
production-build=127
frozen-lockfile=0
```

## Vitest summary

- suites: 152 total, 36 failed
- tests: 351 total, 48 failed

```text
Enhanced Data Table > should handle responsive props
Enhanced Data Table > should hide floating row actions while hovering interactive row controls
Header logout UX > hides the logout action by default
Header logout UX > confirms logout through the desktop hamburger menu
Header mobile overflow prevention > renders header with no horizontal overflow and truncates brand text
NameWithId > shows the id when the name is clicked
NameWithId > does not bubble name clicks into parent row actions
SearchableDropdown > keeps server-provided results when the parent owns the search
SearchableDropdown > filters locally when the caller owns the options
SearchableDropdown > matches local options on their extra keywords
TimestampDisplay > shows the UTC timestamp inside the tooltip
ChannelsPage balance refresh > refreshes a single row balance via per-row icon
ChannelsPage balance refresh > refreshes all balances from header bulk action
ChannelsPage bulk enable/disable > disables all visible channels via bulk action
ChannelsPage priority editor > saves a changed priority on blur
ChannelsPage priority editor > does not call api.put when value is unchanged
ChannelsPage Pagination > should not make duplicate API calls when changing page size
ChannelsPage Pagination > should handle page navigation correctly
ChannelsPage Pagination > should handle sorting without duplicate calls
ChannelsPage Pagination > should duplicate a channel with copied configuration
ChannelsPage Pagination > should show the channel name and type in the delete confirmation dialog
ChannelsPage Pagination > shows an error notification when delete returns success false
ChannelsPage Pagination > shows an error notification when bulk test returns success false
ChannelsPage Pagination > only offers text-compatible testing models and clears to CHEAPEST
ChannelsPage Pagination > filters non-text testing models when the server field is missing
ToolUsageCharts > renders the tool dashboard sections
LogsPage action feedback > shows channel names and reveals channel UUIDs from the channel column
RedemptionsPage action feedback > shows an error when delete returns success false
RedemptionsPage action feedback > shows an error when status update returns success false
TopUpPage: Stripe checkout behavior > submits the exact two-decimal USD amount
TopUpPage: Stripe checkout behavior > blocks amounts below the server-advertised minimum
EditUserPage 2FA disable button > renders disable 2FA button when admin views another user and triggers API call on confirm
UsersPage promote/demote/disable_2fa actions > hides Promote action when current user is not super admin
UsersPage promote/demote/disable_2fa actions > shows Promote when super admin views a regular user
UsersPage promote/demote/disable_2fa actions > promotes a user, updates the row role, and notifies success
UsersPage promote/demote/disable_2fa actions > shows error notification when promote fails
UsersPage promote/demote/disable_2fa actions > demotes an admin and updates the row role
UsersPage promote/demote/disable_2fa actions > shows error notification when demote fails
UsersPage promote/demote/disable_2fa actions > deletes a user row when the backend confirms success
UsersPage promote/demote/disable_2fa actions > shows error notification when delete returns success false
UsersPage promote/demote/disable_2fa actions > shows an error notification when top up returns success false
UsersPage promote/demote/disable_2fa actions > disables 2FA after confirmation and notifies success
UsersPage promote/demote/disable_2fa actions > shows error notification when disable 2FA fails
TokensPage action feedback > shows an error when delete returns success false
TokensPage action feedback > shows an error when status update returns success false
TokensPage share dropdown integration > opens ChatGPT Next Web URL when the dropdown item is selected
TokensPage share dropdown integration > hides ChatGPT Next Web and LobeChat when chat_link is empty
TokensPage share dropdown integration > shows token name and id in the delete confirmation dialog
```

## lint

```text
yarn run v1.22.22
$ eslint .

/Users/runner/work/one-api/one-api/web/modern/eslint.config.js
  37:7  error  Duplicate key 'no-empty'                                 no-dupe-keys
  38:7  error  Duplicate key 'no-useless-assignment'                    no-dupe-keys
  39:7  error  Duplicate key 'prefer-const'                             no-dupe-keys
  40:7  error  Duplicate key '@typescript-eslint/no-empty-object-type'  no-dupe-keys

/Users/runner/work/one-api/one-api/web/modern/src/components/LogDetailsModal.tsx
  245:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u                           @typescript-eslint/no-unused-vars
  261:6   warning  React Hook useEffect has a missing dependency: 'logRef'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/components/Turnstile.tsx
  121:22  warning  The ref value 'containerRef.current' will likely have changed by the time this effect cleanup function runs. If this ref points to a node rendered by React, copy 'containerRef.current' to a variable inside the effect, and use that variable in the cleanup function  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/components/auth/__tests__/ProtectedRoute.test.tsx
  2:10  warning  'BrowserRouter' is defined but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/chat/ImageAttachment.tsx
    6:23  warning  'Eye' is defined but never used. Allowed unused vars must match /^_/u                                              @typescript-eslint/no-unused-vars
    6:46  warning  'Upload' is defined but never used. Allowed unused vars must match /^_/u                                           @typescript-eslint/no-unused-vars
  283:47  warning  'index' is defined but never used. Allowed unused args must match /^_/u                                            @typescript-eslint/no-unused-vars
  327:22  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u                                   @typescript-eslint/no-unused-vars
  335:17  warning  'processingTime' is assigned a value but never used. Allowed unused vars must match /^_/u                          @typescript-eslint/no-unused-vars
  377:22  warning  'fallbackError' is defined but never used. Allowed unused caught errors must match /^_/u                           @typescript-eslint/no-unused-vars
  445:6   warning  React Hook useMemo has a missing dependency: 'handleFileSelect'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/components/chat/MessageItem.tsx
  154:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/dev/responsive-validator.tsx
    1:20  warning  'useEffect' is defined but never used. Allowed unused vars must match /^_/u                  @typescript-eslint/no-unused-vars
    1:31  warning  'useRef' is defined but never used. Allowed unused vars must match /^_/u                     @typescript-eslint/no-unused-vars
    9:47  warning  'Eye' is defined but never used. Allowed unused vars must match /^_/u                        @typescript-eslint/no-unused-vars
    9:60  warning  'Smartphone' is defined but never used. Allowed unused vars must match /^_/u                 @typescript-eslint/no-unused-vars
  146:13  warning  'lowContrastCount' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars
  331:35  warning  'index' is defined but never used. Allowed unused args must match /^_/u                      @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx
  79:6  warning  React Hook useLayoutEffect has a missing dependency: 'updateVisibleItems'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  91:6  warning  React Hook useEffect has a missing dependency: 'updateVisibleItems'. Either include it or remove the dependency array        react-hooks/exhaustive-deps
  93:9  warning  'visibleItems' is assigned a value but never used. Allowed unused vars must match /^_/u                                      @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/theme-provider.tsx
  68:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/adaptive-grid.tsx
   29:11  warning  'isMobile' is assigned a value but never used. Allowed unused vars must match /^_/u      @typescript-eslint/no-unused-vars
   29:21  warning  'isTablet' is assigned a value but never used. Allowed unused vars must match /^_/u      @typescript-eslint/no-unused-vars
  151:49  warning  'minItemWidth' is assigned a value but never used. Allowed unused args must match /^_/u  @typescript-eslint/no-unused-vars
  239:11  warning  'width' is assigned a value but never used. Allowed unused vars must match /^_/u         @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/badge.tsx
  29:17  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/banner.tsx
  112:92  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/confirm-dialog.tsx
  54:17  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/markdown.tsx
   57:21  warning  'node' is defined but never used. Allowed unused args must match /^_/u               @typescript-eslint/no-unused-vars
   63:17  warning  'node' is defined but never used. Allowed unused args must match /^_/u               @typescript-eslint/no-unused-vars
   89:19  warning  'iconSize' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars
  107:18  warning  'node' is defined but never used. Allowed unused args must match /^_/u               @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/notifications.tsx
   53:17  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components
  131:11  warning  'success' is assigned a value but never used. Allowed unused vars must match /^_/u                                              @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/dev/layout-debug.ts
  140:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  142:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  144:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  146:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  148:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  150:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  152:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  154:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
  179:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')

/Users/runner/work/one-api/one-api/web/modern/src/hooks/chat/useChatCompletionStream.ts
  59:24  warning  'jsonParseError' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/hooks/chat/useChatStream.ts
  18:3  warning  'setMessages' is defined but never used. Allowed unused args must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/hooks/usePlaygroundChat.ts
   51:11  warning  'streamResponse' is assigned a value but never used. Allowed unused vars must match /^_/u                                                       @typescript-eslint/no-unused-vars
  183:5   warning  React Hook useCallback has missing dependencies: 'abortControllerRef' and 'setIsStreaming'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  288:5   warning  React Hook useCallback has missing dependencies: 'abortControllerRef' and 'setIsStreaming'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  314:6   warning  React Hook useCallback has a missing dependency: 'abortControllerRef'. Either include it or remove the dependency array                         react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/hooks/useViewport.ts
  230:24  warning  'height' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/HomePage.test.tsx
  19:7  warning  'setLocalStorage' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/about/AboutPage.tsx
  2:29  warning  'CardHeader' is defined but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars
  2:41  warning  'CardTitle' is defined but never used. Allowed unused vars must match /^_/u   @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/GitHubOAuthPage.tsx
  55:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/LarkOAuthPage.tsx
  55:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/LoginPage.impl.tsx
  135:6  warning  React Hook useEffect has a missing dependency: 't'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/OidcOAuthPage.tsx
  54:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/WeChatOAuthPage.tsx
  54:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/__tests__/LoginPage.test.tsx
  4:10  warning  'act' is defined but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/auth/__tests__/LoginPage.turnstile.test.tsx
  124:11  warning  'postSpy' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/channels/ChannelsPage.tsx
  306:6  warning  React Hook useEffect has missing dependencies: 'load', 'pageIndex', and 'pageSize'. Either include them or remove the dependency array                                    react-hooks/exhaustive-deps
  323:6  warning  React Hook useEffect has missing dependencies: 'load', 'pageIndex', 'pageSize', 'performSearch', and 'searchKeyword'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/channels/EditChannelPage.tsx
  2:29  warning  'CardDescription' is defined but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars
  2:46  warning  'CardHeader' is defined but never used. Allowed unused vars must match /^_/u       @typescript-eslint/no-unused-vars
  2:58  warning  'CardTitle' is defined but never used. Allowed unused vars must match /^_/u        @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/channels/components/ChannelModelSettings.tsx
  28:3  warning  'notify' is defined but never used. Allowed unused args must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/chat/PlaygroundPage.tsx
  79:5  warning  'defaultSystemPrompt' is assigned a value but never used. Allowed unused vars must match /^_/u   @typescript-eslint/no-unused-vars
  85:5  warning  'setSelectedModel' is assigned a value but never used. Allowed unused vars must match /^_/u      @typescript-eslint/no-unused-vars
  93:5  warning  'setChannelInputValue' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars
  95:5  warning  'setSelectedChannel' is assigned a value but never used. Allowed unused vars must match /^_/u    @typescript-eslint/no-unused-vars
  97:5  warning  'setModelInputValue' is assigned a value but never used. Allowed unused vars must match /^_/u    @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/chat/hooks/usePlaygroundActions.ts
  71:16  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/chat/hooks/usePlaygroundData.ts
  296:14  warning  'error' is defined but never used. Allowed unused caught errors must match /^_/u                               @typescript-eslint/no-unused-vars
  310:6   warning  React Hook useEffect has a missing dependency: 'loadTokens'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/chat/hooks/usePlaygroundState.ts
    6:34  warning  'useMemo' is defined but never used. Allowed unused vars must match /^_/u                                                                      @typescript-eslint/no-unused-vars
    6:43  warning  'useRef' is defined but never used. Allowed unused vars must match /^_/u                                                                       @typescript-eslint/no-unused-vars
  143:6   warning  React Hook useEffect has missing dependencies: 'defaultSystemPrompt' and 'user?.username'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/chat/playground/hooks/usePlaygroundParameters.ts
  115:3  warning  Unused eslint-disable directive (no problems were reported from 'react-hooks/exhaustive-deps')

/Users/runner/work/one-api/one-api/web/modern/src/pages/dashboard/DashboardPage.tsx
  37:6  warning  React Hook useLayoutEffect has a missing dependency: 'filtersReady'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/dashboard/hooks/useDashboardCharts.ts
  206:5  warning  React Hook useMemo has a missing dependency: 'computeStackedSeries'. Either include it or remove the dependency array      react-hooks/exhaustive-deps
  211:5  warning  React Hook useMemo has a missing dependency: 'computeStackedSeries'. Either include it or remove the dependency array      react-hooks/exhaustive-deps
  221:5  warning  React Hook useMemo has a missing dependency: 'computeStackedSeries'. Either include it or remove the dependency array      react-hooks/exhaustive-deps
  226:5  warning  React Hook useMemo has a missing dependency: 'computeToolStackedSeries'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  231:5  warning  React Hook useMemo has a missing dependency: 'computeToolStackedSeries'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  241:5  warning  React Hook useMemo has a missing dependency: 'computeToolStackedSeries'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  309:5  warning  React Hook useMemo has a missing dependency: 'buildCacheHeatmap'. Either include it or remove the dependency array         react-hooks/exhaustive-deps
  314:5  warning  React Hook useMemo has a missing dependency: 'buildCacheHeatmap'. Either include it or remove the dependency array         react-hooks/exhaustive-deps
  324:5  warning  React Hook useMemo has a missing dependency: 'buildCacheHeatmap'. Either include it or remove the dependency array         react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/dashboard/hooks/useDashboardData.ts
  236:6  warning  React Hook useCallback has a missing dependency: 'validateDateRange'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  243:9  warning  'applyPreset' is assigned a value but never used. Allowed unused vars must match /^_/u                                  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/dashboard/services/chartConfig.tsx
   4:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components
   5:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components
   7:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components
  19:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components
  31:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components

/Users/runner/work/one-api/one-api/web/modern/src/pages/logs/LogsPage.tsx
  139:25  warning  'setSearchKeyword' is assigned a value but never used. Allowed unused vars must match /^_/u                                @typescript-eslint/no-unused-vars
  140:10  warning  'searchOptions' is assigned a value but never used. Allowed unused vars must match /^_/u                                   @typescript-eslint/no-unused-vars
  141:10  warning  'searchLoading' is assigned a value but never used. Allowed unused vars must match /^_/u                                   @typescript-eslint/no-unused-vars
  245:9   warning  'searchLogs' is assigned a value but never used. Allowed unused vars must match /^_/u                                      @typescript-eslint/no-unused-vars
  317:6   warning  React Hook useEffect has missing dependencies: 'load' and 'pageIndex'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  323:6   warning  React Hook useEffect has a missing dependency: 'loadStatistics'. Either include it or remove the dependency array          react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/mcp/EditMCPServerPage.tsx
  141:6  warning  React Hook useEffect has missing dependencies: 'isEdit', 'loadServer', and 'loadTools'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/mcp/MCPServersPage.tsx
  200:5  warning  React Hook useMemo has missing dependencies: 'deleteServer', 'navigate', 'syncServer', and 'testServer'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  407:6  warning  React Hook useEffect has a missing dependency: 'load'. Either include it or remove the dependency array                                                      react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/models/ModelPricingModal.tsx
  1177:10  warning  'formatTokenCount' is defined but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/pages/models/ModelsPage.tsx
  128:6  warning  React Hook useEffect has missing dependencies: 'selectedModel?.name' and 'setSearchParams'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/redemptions/EditRedemptionPage.tsx
  146:6  warning  React Hook useEffect has a missing dependency: 'loadRedemption'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/redemptions/RedemptionsPage.tsx
  128:6  warning  React Hook useEffect has missing dependencies: 'load', 'pageIndex', 'pageSize', 'search', and 'searchKeyword'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/settings/OperationSettings.tsx
  294:6  warning  React Hook useEffect has a missing dependency: 'loadOptions'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/settings/OtherSettings.tsx
  141:6  warning  React Hook useEffect has a missing dependency: 'loadOptions'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/settings/PersonalSettings.tsx
  204:6  warning  React Hook useEffect has missing dependencies: 'loadPasskeys', 'loadProfile', and 'loadTotpStatus'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/EditTokenPage.tsx
    3:29  warning  'CardDescription' is defined but never used. Allowed unused vars must match /^_/u                             @typescript-eslint/no-unused-vars
    3:46  warning  'CardHeader' is defined but never used. Allowed unused vars must match /^_/u                                  @typescript-eslint/no-unused-vars
    3:58  warning  'CardTitle' is defined but never used. Allowed unused vars must match /^_/u                                   @typescript-eslint/no-unused-vars
   96:9   warning  'watchRemainQuota' is assigned a value but never used. Allowed unused vars must match /^_/u                   @typescript-eslint/no-unused-vars
  174:6   warning  React Hook useEffect has a missing dependency: 'loadToken'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/TokensPage.impl.tsx
   66:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components                                            react-refresh/only-export-components
   97:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components                                            react-refresh/only-export-components
  115:14  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components                                            react-refresh/only-export-components
  248:6   warning  React Hook useEffect has missing dependencies: 'load', 'pageIndex', 'pageSize', 'performSearch', and 'searchKeyword'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  259:6   warning  React Hook useEffect has missing dependencies: 'load', 'pageIndex', 'pageSize', 'performSearch', and 'searchKeyword'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/tokens/useClipboardManager.ts
  60:36  warning  The ref value 'resetTimersRef.current' will likely have changed by the time this effect cleanup function runs. If this ref points to a node rendered by React, copy 'resetTimersRef.current' to a variable inside the effect, and use that variable in the cleanup function  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/topup/TopUpPage.tsx
  263:6  warning  React Hook useCallback has a missing dependency: 'loadUserData'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/users/EditUserPage.tsx
  204:6  warning  React Hook useEffect has a missing dependency: 'loadUser'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/users/UsersPage.tsx
  210:6  warning  React Hook useEffect has missing dependencies: 'load', 'pageIndex', 'pageSize', 'search', and 'searchKeyword'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/test/setup.ts
   89:3  warning  Unused eslint-disable directive (no problems were reported from '@typescript-eslint/no-empty-function')
   91:3  warning  Unused eslint-disable directive (no problems were reported from '@typescript-eslint/no-empty-function')
  111:3  warning  Unused eslint-disable directive (no problems were reported from '@typescript-eslint/no-empty-function')

✖ 133 problems (4 errors, 129 warnings)
  0 errors and 13 warnings potentially fixable with the `--fix` option.

error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## production-build

```text
yarn run v1.22.22
$ tsgo -b && vite build --mode production --emptyOutDir
/bin/sh: tsgo: command not found
error Command failed with exit code 127.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## type-check

```text
yarn run v1.22.22
$ tsgo --noEmit
/bin/sh: tsgo: command not found
error Command failed with exit code 127.
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
