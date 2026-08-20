# Modern latest-stable compatibility diagnostics

- Resolver base: latest `main` at staging reset
- Node: `24.x`
- Yarn: `1.22.22`

## Check status

```text
current=0
type-check=1
lint=1
i18n=0
unit-tests=1
production-build=1
frozen-lockfile=0
```

## lint

```text
yarn run v1.22.22
$ eslint .

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

/Users/runner/work/one-api/one-api/web/modern/src/components/dev/responsive-debugger.tsx
  180:3  error  React Hook "useEffect" is called conditionally. React Hooks must be called in the exact same order in every component render  react-hooks/rules-of-hooks

/Users/runner/work/one-api/one-api/web/modern/src/components/dev/responsive-validator.tsx
    1:20  warning  'useEffect' is defined but never used. Allowed unused vars must match /^_/u                  @typescript-eslint/no-unused-vars
    1:31  warning  'useRef' is defined but never used. Allowed unused vars must match /^_/u                     @typescript-eslint/no-unused-vars
    9:47  warning  'Eye' is defined but never used. Allowed unused vars must match /^_/u                        @typescript-eslint/no-unused-vars
    9:60  warning  'Smartphone' is defined but never used. Allowed unused vars must match /^_/u                 @typescript-eslint/no-unused-vars
  146:13  error    'lowContrastCount' is never reassigned. Use 'const' instead                                  prefer-const
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
  54:17  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components                            react-refresh/only-export-components
  72:19  error    React Hook "useTranslation" cannot be called inside a callback. React Hooks must be called in a React function component or a custom React Hook function  react-hooks/rules-of-hooks

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/input.tsx
  4:18  error  An interface declaring no members is equivalent to its supertype  @typescript-eslint/no-empty-object-type

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/markdown.tsx
   57:21  warning  'node' is defined but never used. Allowed unused args must match /^_/u               @typescript-eslint/no-unused-vars
   63:17  warning  'node' is defined but never used. Allowed unused args must match /^_/u               @typescript-eslint/no-unused-vars
   89:19  warning  'iconSize' is assigned a value but never used. Allowed unused vars must match /^_/u  @typescript-eslint/no-unused-vars
  107:18  warning  'node' is defined but never used. Allowed unused args must match /^_/u               @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/notifications.tsx
   53:17  warning  Fast refresh only works when a file only exports components. Use a new file to share constants or functions between components  react-refresh/only-export-components
  131:11  warning  'success' is assigned a value but never used. Allowed unused vars must match /^_/u                                              @typescript-eslint/no-unused-vars

/Users/runner/work/one-api/one-api/web/modern/src/components/ui/textarea.tsx
  5:18  error  An interface declaring no members is equivalent to its supertype  @typescript-eslint/no-empty-object-type

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
   18:3   warning  'setMessages' is defined but never used. Allowed unused args must match /^_/u  @typescript-eslint/no-unused-vars
  261:11  error    The value assigned to 'reachedEnd' is not used in subsequent statements        no-useless-assignment
  269:13  error    The value assigned to 'reachedEnd' is not used in subsequent statements        no-useless-assignment

/Users/runner/work/one-api/one-api/web/modern/src/hooks/usePlaygroundChat.ts
   51:11  warning  'streamResponse' is assigned a value but never used. Allowed unused vars must match /^_/u                                                       @typescript-eslint/no-unused-vars
  183:5   warning  React Hook useCallback has missing dependencies: 'abortControllerRef' and 'setIsStreaming'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  288:5   warning  React Hook useCallback has missing dependencies: 'abortControllerRef' and 'setIsStreaming'. Either include them or remove the dependency array  react-hooks/exhaustive-deps
  314:6   warning  React Hook useCallback has a missing dependency: 'abortControllerRef'. Either include it or remove the dependency array                         react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/hooks/usePlaygroundChat/useStreamResponse.ts
  133:37  error  Empty block statement                                                    no-empty
  261:11  error  The value assigned to 'reachedEnd' is not used in subsequent statements  no-useless-assignment
  269:13  error  The value assigned to 'reachedEnd' is not used in subsequent statements  no-useless-assignment

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
   50:9   error    The value assigned to 'savedMessages' is not used in subsequent statements                                                                     no-useless-assignment
   51:9   error    The value assigned to 'savedConversationId' is not used in subsequent statements                                                               no-useless-assignment
   52:9   error    The value assigned to 'savedConversationCreated' is not used in subsequent statements                                                          no-useless-assignment
   53:9   error    The value assigned to 'savedConversationCreatedBy' is not used in subsequent statements                                                        no-useless-assignment
  143:6   warning  React Hook useEffect has missing dependencies: 'defaultSystemPrompt' and 'user?.username'. Either include them or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/chat/playground/hooks/usePlaygroundParameters.ts
  115:3  warning  Unused eslint-disable directive (no problems were reported from 'react-hooks/exhaustive-deps')

/Users/runner/work/one-api/one-api/web/modern/src/pages/dashboard/DashboardPage.tsx
  37:6  warning  React Hook useLayoutEffect has a missing dependency: 'filtersReady'. Either include it or remove the dependency array  react-hooks/exhaustive-deps

/Users/runner/work/one-api/one-api/web/modern/src/pages/dashboard/hooks/useDashboardCharts.ts
  206:5   warning  React Hook useMemo has a missing dependency: 'computeStackedSeries'. Either include it or remove the dependency array      react-hooks/exhaustive-deps
  211:5   warning  React Hook useMemo has a missing dependency: 'computeStackedSeries'. Either include it or remove the dependency array      react-hooks/exhaustive-deps
  221:5   warning  React Hook useMemo has a missing dependency: 'computeStackedSeries'. Either include it or remove the dependency array      react-hooks/exhaustive-deps
  226:5   warning  React Hook useMemo has a missing dependency: 'computeToolStackedSeries'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  231:5   warning  React Hook useMemo has a missing dependency: 'computeToolStackedSeries'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  241:5   warning  React Hook useMemo has a missing dependency: 'computeToolStackedSeries'. Either include it or remove the dependency array  react-hooks/exhaustive-deps
  262:11  error    The value assigned to 'numerator' is not used in subsequent statements                                                     no-useless-assignment
  263:11  error    The value assigned to 'denominator' is not used in subsequent statements                                                   no-useless-assignment
  309:5   warning  React Hook useMemo has a missing dependency: 'buildCacheHeatmap'. Either include it or remove the dependency array         react-hooks/exhaustive-deps
  314:5   warning  React Hook useMemo has a missing dependency: 'buildCacheHeatmap'. Either include it or remove the dependency array         react-hooks/exhaustive-deps
  324:5   warning  React Hook useMemo has a missing dependency: 'buildCacheHeatmap'. Either include it or remove the dependency array         react-hooks/exhaustive-deps

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
  209:11  error    'payload' is never reassigned. Use 'const' instead                                                            prefer-const
  365:5   error    Definition for rule 'react/jsx-no-useless-fragment' was not found                                             react/jsx-no-useless-fragment

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
    3:8  warning  'enTranslations' is defined but never used. Allowed unused vars must match /^_/u                                     @typescript-eslint/no-unused-vars
   88:3  warning  Unused eslint-disable directive (no problems were reported from '@typescript-eslint/no-empty-function')
   90:3  warning  Unused eslint-disable directive (no problems were reported from '@typescript-eslint/no-empty-function')
  104:3  error    Use "@ts-expect-error" instead of "@ts-ignore", as "@ts-ignore" will do nothing if the following line is error-free  @typescript-eslint/ban-ts-comment
  110:3  warning  Unused eslint-disable directive (no problems were reported from '@typescript-eslint/no-empty-function')

✖ 149 problems (19 errors, 130 warnings)
  2 errors and 13 warnings potentially fixable with the `--fix` option.

error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## production-build

```text
yarn run v1.22.22
$ tsc -b && vite build --mode production --emptyOutDir
node_modules/@testing-library/jest-dom/types/jest.d.ts(1,23): error TS2688: Cannot find type definition file for 'jest'.
src/components/chat/MessageList.tsx(91,15): error TS2322: Type '((messageIndex: number, newContent: string) => void) | undefined' is not assignable to type '((messageIndex: number, newContent: string | any[]) => void) | undefined'.
  Type '(messageIndex: number, newContent: string) => void' is not assignable to type '(messageIndex: number, newContent: string | any[]) => void'.
    Types of parameters 'newContent' and 'newContent' are incompatible.
      Type 'string | any[]' is not assignable to type 'string'.
        Type 'any[]' is not assignable to type 'string'.
src/components/layout/Header.tsx(70,19): error TS18048: 'user.role' is possibly 'undefined'.
src/components/layout/HeaderNav.tsx(103,13): error TS2322: Type '(el: HTMLAnchorElement | null) => HTMLAnchorElement | null' is not assignable to type 'Ref<HTMLAnchorElement> | undefined'.
  Type '(el: HTMLAnchorElement | null) => HTMLAnchorElement | null' is not assignable to type '(instance: HTMLAnchorElement | null) => void | (() => VoidOrUndefinedOnly)'.
    Type 'HTMLAnchorElement | null' is not assignable to type 'void | (() => VoidOrUndefinedOnly)'.
      Type 'null' is not assignable to type 'void | (() => VoidOrUndefinedOnly)'.
src/components/shared/NameWithId.tsx(34,24): error TS2554: Expected 1 arguments, but got 0.
src/components/ui/data-table.tsx(2,22): error TS2724: '"@tanstack/react-table"' has no exported member named 'getCoreRowModel'. Did you mean 'createCoreRowModel'?
src/components/ui/data-table.tsx(2,39): error TS2724: '"@tanstack/react-table"' has no exported member named 'useReactTable'. Did you mean 'ReactTable'?
src/components/ui/data-table.tsx(13,22): error TS2344: Type 'TData' does not satisfy the constraint 'TableFeatures'.
src/components/ui/data-table.tsx(89,20): error TS2344: Type 'TData' does not satisfy the constraint 'TableFeatures'.
src/components/ui/data-table.tsx(121,45): error TS7006: Parameter 'row' implicitly has an 'any' type.
src/components/ui/data-table.tsx(123,47): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/data-table.tsx(149,47): error TS7006: Parameter 'headerGroup' implicitly has an 'any' type.
src/components/ui/data-table.tsx(151,47): error TS7006: Parameter 'header' implicitly has an 'any' type.
src/components/ui/data-table.tsx(161,49): error TS7006: Parameter 'row' implicitly has an 'any' type.
src/components/ui/data-table.tsx(163,51): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/enhanced-data-table.tsx(18,22): error TS2344: Type 'TData' does not satisfy the constraint 'RowData'.
  Type 'TData' is not assignable to type 'any[]'.
src/components/ui/enhanced-data-table.tsx(103,33): error TS2554: Expected 1 arguments, but got 0.
src/components/ui/enhanced-data-table.tsx(213,20): error TS2344: Type 'TData' does not satisfy the constraint 'RowData'.
  Type 'TData' is not assignable to type 'any[]'.
src/components/ui/enhanced-data-table.tsx(217,5): error TS2322: Type 'TData[]' is not assignable to type 'readonly RowData[]'.
  Type 'TData' is not assignable to type 'RowData'.
    Type 'TData' is not assignable to type 'any[]'.
src/components/ui/enhanced-data-table.tsx(218,5): error TS2322: Type 'LegacyColumnDef<TData, TValue>[]' is not assignable to type 'readonly ColumnDef<LegacyFeatures, RowData, unknown>[]'.
  Type 'LegacyColumnDef<TData, TValue>' is not assignable to type 'ColumnDef<LegacyFeatures, RowData, unknown>'.
    Type 'ColumnDefBase_Core<LegacyFeatures, TData, TValue> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, TData, TValue> & ... 9 more ... & IdIdentifier<...>' is not assignable to type 'ColumnDef<LegacyFeatures, RowData, unknown>'.
      Type 'ColumnDefBase_Core<LegacyFeatures, TData, TValue> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, TData, TValue> & ... 9 more ... & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<LegacyFeatures, RowData, unknown> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, RowData, unknown> & ... 10 more ... & Partial<...>'.
        Type 'ColumnDefBase_Core<LegacyFeatures, TData, TValue> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, TData, TValue> & ... 9 more ... & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<LegacyFeatures, RowData, unknown>'.
          Types of property 'getUniqueValues' are incompatible.
            Type 'AccessorFn<TData, readonly unknown[]> | undefined' is not assignable to type 'AccessorFn<RowData, readonly unknown[]> | undefined'.
              Type 'AccessorFn<TData, readonly unknown[]>' is not assignable to type 'AccessorFn<RowData, readonly unknown[]>'.
                Type 'RowData' is not assignable to type 'TData'.
                  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(332,47): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(393,53): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(394,69): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(395,67): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/markdown.tsx(15,8): error TS2882: Cannot find module or type declarations for side-effect import of 'highlight.js/styles/github.css'.
src/components/ui/markdown.tsx(16,8): error TS2882: Cannot find module or type declarations for side-effect import of 'katex/dist/katex.min.css'.
src/components/ui/searchable-dropdown.tsx(78,34): error TS2554: Expected 1 arguments, but got 0.
src/hooks/__tests__/useRealtimeChat.test.ts(81,26): error TS2558: Expected 0-1 type arguments, but got 2.
src/hooks/__tests__/useRealtimeChat.test.ts(88,5): error TS2322: Type '((entry: AddEventInput) => void) | Mock<[AddEventInput]>' is not assignable to type '(entry: AddEventInput) => void'.
  Type 'Mock<[AddEventInput]>' is not assignable to type '(entry: AddEventInput) => void'.
    Types of parameters 'args' and 'entry' are incompatible.
      Type '[entry: AddEventInput]' is not assignable to type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(146,8): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(146,26): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(146,62): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(146,90): error TS2339: Property 'transport' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(243,47): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(243,65): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(243,94): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(287,48): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(287,66): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(287,95): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(337,49): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(337,67): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(337,104): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(349,51): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(349,69): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(365,51): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(365,69): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(401,53): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(401,71): error TS2339: Property 'type' does not exist on type 'never'.
src/lib/__tests__/api.test.ts(100,29): error TS2322: Type 'Mock<Constructable | Procedure>' is not assignable to type '(() => void) | undefined'.
  Type 'MockInstance<Constructable | Procedure> & (new (...args: any[]) => any) & {}' is not assignable to type '(() => void) | undefined'.
src/main.tsx(6,8): error TS2882: Cannot find module or type declarations for side-effect import of './index.css'.
src/pages/NotFoundPage.tsx(12,19): error TS2554: Expected 1 arguments, but got 0.
src/pages/NotFoundPage.tsx(13,20): error TS2554: Expected 1 arguments, but got 0.
src/pages/channels/components/ChannelAdvancedSettings.tsx(26,34): error TS2345: Argument of type 'string | undefined' is not assignable to parameter of type 'string'.
  Type 'undefined' is not assignable to type 'string'.
src/pages/channels/components/ChannelAdvancedSettings.tsx(32,34): error TS2345: Argument of type 'string | undefined' is not assignable to parameter of type 'string'.
  Type 'undefined' is not assignable to type 'string'.
src/pages/channels/components/ChannelEndpointSettings.tsx(235,17): error TS2345: Argument of type '{}' is not assignable to parameter of type 'SetStateAction<string>'.
src/pages/channels/components/ChannelEndpointSettings.tsx(412,68): error TS2339: Property 'trim' does not exist on type '{}'.
src/pages/channels/components/ChannelEndpointSettings.tsx(452,23): error TS2322: Type 'unknown' is not assignable to type 'Iterable<ReactI18NextChildren> | ReactI18NextChildren'.
src/pages/channels/components/__tests__/ChannelModelSettings.test.tsx(123,21): error TS2339: Property 'getValues' does not exist on type 'never'.
src/pages/channels/schemas.ts(47,26): error TS2554: Expected 2-3 arguments, but got 1.
src/pages/channels/schemas.ts(49,27): error TS2554: Expected 2-3 arguments, but got 1.
src/pages/channels/schemas.ts(59,16): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Argument of type '{}' is not assignable to parameter of type '() => { region?: string | undefined; ak?: string | undefined; sk?: string | undefined; user_id?: string | undefined; vertex_ai_project_id?: string | undefined; vertex_ai_adc?: string | undefined; ... 11 more ...; tencent_secret_key?: string | undefined; }'.
      Type '{}' provides no match for the signature '(): { region?: string | undefined; ak?: string | undefined; sk?: string | undefined; user_id?: string | undefined; vertex_ai_project_id?: string | undefined; vertex_ai_adc?: string | undefined; ... 11 more ...; tencent_secret_key?: string | undefined; }'.
src/pages/chat/PlaygroundPage.tsx(12,8): error TS2882: Cannot find module or type declarations for side-effect import of 'highlight.js/styles/a11y-dark.css'.
src/pages/chat/PlaygroundPage.tsx(13,8): error TS2882: Cannot find module or type declarations for side-effect import of 'katex/dist/katex.min.css'.
src/pages/chat/PlaygroundPage.tsx(178,9): error TS2322: Type '(message: string, images?: ImageAttachment[] | undefined) => Promise<void> | undefined' is not assignable to type '(message: string, images?: ImageAttachmentType[]) => Promise<void>'.
  Type 'Promise<void> | undefined' is not assignable to type 'Promise<void>'.
    Type 'undefined' is not assignable to type 'Promise<void>'.
src/pages/dashboard/components/UsageSections.tsx(72,16): error TS2322: Type 'ContentType<number, string> | undefined' is not assignable to type 'ContentType<ValueType, NameType> | undefined'.
  Type '(props: TooltipContentProps<number, string>) => ReactNode' is not assignable to type 'ContentType<ValueType, NameType> | undefined'.
    Type '(props: TooltipContentProps<number, string>) => ReactNode' is not assignable to type '(props: TooltipContentProps<ValueType, NameType>) => ReactNode'.
      Types of parameters 'props' and 'props' are incompatible.
        Type 'TooltipContentProps<ValueType, NameType>' is not assignable to type 'TooltipContentProps<number, string>'.
          Type 'TooltipContentProps<ValueType, NameType>' is not assignable to type '{ active?: boolean | undefined; allowEscapeViewBox?: AllowInDimension | undefined; animationDuration?: number | undefined; animationEasing?: EasingInput | undefined; ... 22 more ...; wrapperStyle?: CSSProperties | undefined; }'.
            Types of property 'content' are incompatible.
              Type 'ContentType<ValueType, NameType> | undefined' is not assignable to type 'ContentType<number, string> | undefined'.
                Type '(props: TooltipContentProps<ValueType, NameType>) => ReactNode' is not assignable to type 'ContentType<number, string> | undefined'.
                  Type '(props: TooltipContentProps<ValueType, NameType>) => ReactNode' is not assignable to type '(props: TooltipContentProps<number, string>) => ReactNode'.
                    Types of parameters 'props' and 'props' are incompatible.
                      Type 'TooltipContentProps<number, string>' is not assignable to type 'TooltipContentProps<ValueType, NameType>'.
                        Type 'TooltipContentProps<number, string>' is not assignable to type 'Omit<Props<ValueType, NameType>, PropertiesReadFromContext>'.
                          Types of property 'formatter' are incompatible.
                            Type '(Formatter<number, string> & ((value: number, name: string, item: TooltipPayloadEntry, index: number, payload: TooltipPayload) => [...] | ReactNode)) | undefined' is not assignable to type 'Formatter<ValueType, NameType> | undefined'.
                              Type 'Formatter<number, string> & ((value: number, name: string, item: TooltipPayloadEntry, index: number, payload: TooltipPayload) => [...] | ReactNode)' is not assignable to type 'Formatter<ValueType, NameType> | undefined'.
                                Type 'Formatter<number, string> & ((value: number, name: string, item: TooltipPayloadEntry, index: number, payload: TooltipPayload) => [...] | ReactNode)' is not assignable to type 'Formatter<ValueType, NameType>'.
                                  Types of parameters 'value' and 'value' are incompatible.
                                    Type 'ValueType | undefined' is not assignable to type 'number | undefined'.
                                      Type 'string' is not assignable to type 'number'.
src/pages/redemptions/RedemptionsPage.tsx(313,13): error TS2322: Type 'RedemptionRow[]' is not assignable to type 'LegacyFeatures[]'.
  Type 'RedemptionRow' is missing the following properties from type 'LegacyFeatures': aggregationFns, filterFns, sortFns, cellSelectionFeature, and 16 more.
src/pages/settings/SettingsPage.tsx(23,18): error TS18048: 'user.role' is possibly 'undefined'.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## type-check

```text
yarn run v1.22.22
$ tsc --noEmit
node_modules/@testing-library/jest-dom/types/jest.d.ts(1,23): error TS2688: Cannot find type definition file for 'jest'.
src/components/chat/MessageList.tsx(91,15): error TS2322: Type '((messageIndex: number, newContent: string) => void) | undefined' is not assignable to type '((messageIndex: number, newContent: string | any[]) => void) | undefined'.
  Type '(messageIndex: number, newContent: string) => void' is not assignable to type '(messageIndex: number, newContent: string | any[]) => void'.
    Types of parameters 'newContent' and 'newContent' are incompatible.
      Type 'string | any[]' is not assignable to type 'string'.
        Type 'any[]' is not assignable to type 'string'.
src/components/layout/Header.tsx(70,19): error TS18048: 'user.role' is possibly 'undefined'.
src/components/layout/HeaderNav.tsx(103,13): error TS2322: Type '(el: HTMLAnchorElement | null) => HTMLAnchorElement | null' is not assignable to type 'Ref<HTMLAnchorElement> | undefined'.
  Type '(el: HTMLAnchorElement | null) => HTMLAnchorElement | null' is not assignable to type '(instance: HTMLAnchorElement | null) => void | (() => VoidOrUndefinedOnly)'.
    Type 'HTMLAnchorElement | null' is not assignable to type 'void | (() => VoidOrUndefinedOnly)'.
      Type 'null' is not assignable to type 'void | (() => VoidOrUndefinedOnly)'.
src/components/shared/NameWithId.tsx(34,24): error TS2554: Expected 1 arguments, but got 0.
src/components/ui/data-table.tsx(2,22): error TS2724: '"@tanstack/react-table"' has no exported member named 'getCoreRowModel'. Did you mean 'createCoreRowModel'?
src/components/ui/data-table.tsx(2,39): error TS2724: '"@tanstack/react-table"' has no exported member named 'useReactTable'. Did you mean 'ReactTable'?
src/components/ui/data-table.tsx(13,22): error TS2344: Type 'TData' does not satisfy the constraint 'TableFeatures'.
src/components/ui/data-table.tsx(89,20): error TS2344: Type 'TData' does not satisfy the constraint 'TableFeatures'.
src/components/ui/data-table.tsx(121,45): error TS7006: Parameter 'row' implicitly has an 'any' type.
src/components/ui/data-table.tsx(123,47): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/data-table.tsx(149,47): error TS7006: Parameter 'headerGroup' implicitly has an 'any' type.
src/components/ui/data-table.tsx(151,47): error TS7006: Parameter 'header' implicitly has an 'any' type.
src/components/ui/data-table.tsx(161,49): error TS7006: Parameter 'row' implicitly has an 'any' type.
src/components/ui/data-table.tsx(163,51): error TS7006: Parameter 'cell' implicitly has an 'any' type.
src/components/ui/enhanced-data-table.tsx(18,22): error TS2344: Type 'TData' does not satisfy the constraint 'RowData'.
  Type 'TData' is not assignable to type 'any[]'.
src/components/ui/enhanced-data-table.tsx(103,33): error TS2554: Expected 1 arguments, but got 0.
src/components/ui/enhanced-data-table.tsx(213,20): error TS2344: Type 'TData' does not satisfy the constraint 'RowData'.
  Type 'TData' is not assignable to type 'any[]'.
src/components/ui/enhanced-data-table.tsx(217,5): error TS2322: Type 'TData[]' is not assignable to type 'readonly RowData[]'.
  Type 'TData' is not assignable to type 'RowData'.
    Type 'TData' is not assignable to type 'any[]'.
src/components/ui/enhanced-data-table.tsx(218,5): error TS2322: Type 'LegacyColumnDef<TData, TValue>[]' is not assignable to type 'readonly ColumnDef<LegacyFeatures, RowData, unknown>[]'.
  Type 'LegacyColumnDef<TData, TValue>' is not assignable to type 'ColumnDef<LegacyFeatures, RowData, unknown>'.
    Type 'ColumnDefBase_Core<LegacyFeatures, TData, TValue> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, TData, TValue> & ... 9 more ... & IdIdentifier<...>' is not assignable to type 'ColumnDef<LegacyFeatures, RowData, unknown>'.
      Type 'ColumnDefBase_Core<LegacyFeatures, TData, TValue> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, TData, TValue> & ... 9 more ... & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<LegacyFeatures, RowData, unknown> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, RowData, unknown> & ... 10 more ... & Partial<...>'.
        Type 'ColumnDefBase_Core<LegacyFeatures, TData, TValue> & ColumnDef_CellSelection & ColumnDef_CellSpanning<LegacyFeatures, TData, TValue> & ... 9 more ... & IdIdentifier<...>' is not assignable to type 'ColumnDefBase_Core<LegacyFeatures, RowData, unknown>'.
          Types of property 'getUniqueValues' are incompatible.
            Type 'AccessorFn<TData, readonly unknown[]> | undefined' is not assignable to type 'AccessorFn<RowData, readonly unknown[]> | undefined'.
              Type 'AccessorFn<TData, readonly unknown[]>' is not assignable to type 'AccessorFn<RowData, readonly unknown[]>'.
                Type 'RowData' is not assignable to type 'TData'.
                  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(332,47): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(393,53): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(394,69): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/enhanced-data-table.tsx(395,67): error TS2345: Argument of type 'RowData' is not assignable to parameter of type 'TData'.
  'TData' could be instantiated with an arbitrary type which could be unrelated to 'RowData'.
src/components/ui/markdown.tsx(15,8): error TS2882: Cannot find module or type declarations for side-effect import of 'highlight.js/styles/github.css'.
src/components/ui/markdown.tsx(16,8): error TS2882: Cannot find module or type declarations for side-effect import of 'katex/dist/katex.min.css'.
src/components/ui/searchable-dropdown.tsx(78,34): error TS2554: Expected 1 arguments, but got 0.
src/hooks/__tests__/useRealtimeChat.test.ts(81,26): error TS2558: Expected 0-1 type arguments, but got 2.
src/hooks/__tests__/useRealtimeChat.test.ts(88,5): error TS2322: Type '((entry: AddEventInput) => void) | Mock<[AddEventInput]>' is not assignable to type '(entry: AddEventInput) => void'.
  Type 'Mock<[AddEventInput]>' is not assignable to type '(entry: AddEventInput) => void'.
    Types of parameters 'args' and 'entry' are incompatible.
      Type '[entry: AddEventInput]' is not assignable to type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(146,8): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(146,26): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(146,62): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(146,90): error TS2339: Property 'transport' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(243,47): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(243,65): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(243,94): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(287,48): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(287,66): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(287,95): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(337,49): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(337,67): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(337,104): error TS2339: Property 'direction' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(349,51): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(349,69): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(365,51): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(365,69): error TS2339: Property 'type' does not exist on type 'never'.
src/hooks/__tests__/useRealtimeChat.test.ts(401,53): error TS2488: Type 'never' must have a '[Symbol.iterator]()' method that returns an iterator.
src/hooks/__tests__/useRealtimeChat.test.ts(401,71): error TS2339: Property 'type' does not exist on type 'never'.
src/lib/__tests__/api.test.ts(100,29): error TS2322: Type 'Mock<Constructable | Procedure>' is not assignable to type '(() => void) | undefined'.
  Type 'MockInstance<Constructable | Procedure> & (new (...args: any[]) => any) & {}' is not assignable to type '(() => void) | undefined'.
src/main.tsx(6,8): error TS2882: Cannot find module or type declarations for side-effect import of './index.css'.
src/pages/NotFoundPage.tsx(12,19): error TS2554: Expected 1 arguments, but got 0.
src/pages/NotFoundPage.tsx(13,20): error TS2554: Expected 1 arguments, but got 0.
src/pages/channels/components/ChannelAdvancedSettings.tsx(26,34): error TS2345: Argument of type 'string | undefined' is not assignable to parameter of type 'string'.
  Type 'undefined' is not assignable to type 'string'.
src/pages/channels/components/ChannelAdvancedSettings.tsx(32,34): error TS2345: Argument of type 'string | undefined' is not assignable to parameter of type 'string'.
  Type 'undefined' is not assignable to type 'string'.
src/pages/channels/components/ChannelEndpointSettings.tsx(235,17): error TS2345: Argument of type '{}' is not assignable to parameter of type 'SetStateAction<string>'.
src/pages/channels/components/ChannelEndpointSettings.tsx(412,68): error TS2339: Property 'trim' does not exist on type '{}'.
src/pages/channels/components/ChannelEndpointSettings.tsx(452,23): error TS2322: Type 'unknown' is not assignable to type 'Iterable<ReactI18NextChildren> | ReactI18NextChildren'.
src/pages/channels/components/__tests__/ChannelModelSettings.test.tsx(123,21): error TS2339: Property 'getValues' does not exist on type 'never'.
src/pages/channels/schemas.ts(47,26): error TS2554: Expected 2-3 arguments, but got 1.
src/pages/channels/schemas.ts(49,27): error TS2554: Expected 2-3 arguments, but got 1.
src/pages/channels/schemas.ts(59,16): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Argument of type '{}' is not assignable to parameter of type '() => { region?: string | undefined; ak?: string | undefined; sk?: string | undefined; user_id?: string | undefined; vertex_ai_project_id?: string | undefined; vertex_ai_adc?: string | undefined; ... 11 more ...; tencent_secret_key?: string | undefined; }'.
      Type '{}' provides no match for the signature '(): { region?: string | undefined; ak?: string | undefined; sk?: string | undefined; user_id?: string | undefined; vertex_ai_project_id?: string | undefined; vertex_ai_adc?: string | undefined; ... 11 more ...; tencent_secret_key?: string | undefined; }'.
src/pages/chat/PlaygroundPage.tsx(12,8): error TS2882: Cannot find module or type declarations for side-effect import of 'highlight.js/styles/a11y-dark.css'.
src/pages/chat/PlaygroundPage.tsx(13,8): error TS2882: Cannot find module or type declarations for side-effect import of 'katex/dist/katex.min.css'.
src/pages/chat/PlaygroundPage.tsx(178,9): error TS2322: Type '(message: string, images?: ImageAttachment[] | undefined) => Promise<void> | undefined' is not assignable to type '(message: string, images?: ImageAttachmentType[]) => Promise<void>'.
  Type 'Promise<void> | undefined' is not assignable to type 'Promise<void>'.
    Type 'undefined' is not assignable to type 'Promise<void>'.
src/pages/dashboard/components/UsageSections.tsx(72,16): error TS2322: Type 'ContentType<number, string> | undefined' is not assignable to type 'ContentType<ValueType, NameType> | undefined'.
  Type '(props: TooltipContentProps<number, string>) => ReactNode' is not assignable to type 'ContentType<ValueType, NameType> | undefined'.
    Type '(props: TooltipContentProps<number, string>) => ReactNode' is not assignable to type '(props: TooltipContentProps<ValueType, NameType>) => ReactNode'.
      Types of parameters 'props' and 'props' are incompatible.
        Type 'TooltipContentProps<ValueType, NameType>' is not assignable to type 'TooltipContentProps<number, string>'.
          Type 'TooltipContentProps<ValueType, NameType>' is not assignable to type '{ active?: boolean | undefined; allowEscapeViewBox?: AllowInDimension | undefined; animationDuration?: number | undefined; animationEasing?: EasingInput | undefined; ... 22 more ...; wrapperStyle?: CSSProperties | undefined; }'.
            Types of property 'content' are incompatible.
              Type 'ContentType<ValueType, NameType> | undefined' is not assignable to type 'ContentType<number, string> | undefined'.
                Type '(props: TooltipContentProps<ValueType, NameType>) => ReactNode' is not assignable to type 'ContentType<number, string> | undefined'.
                  Type '(props: TooltipContentProps<ValueType, NameType>) => ReactNode' is not assignable to type '(props: TooltipContentProps<number, string>) => ReactNode'.
                    Types of parameters 'props' and 'props' are incompatible.
                      Type 'TooltipContentProps<number, string>' is not assignable to type 'TooltipContentProps<ValueType, NameType>'.
                        Type 'TooltipContentProps<number, string>' is not assignable to type 'Omit<Props<ValueType, NameType>, PropertiesReadFromContext>'.
                          Types of property 'formatter' are incompatible.
                            Type '(Formatter<number, string> & ((value: number, name: string, item: TooltipPayloadEntry, index: number, payload: TooltipPayload) => [...] | ReactNode)) | undefined' is not assignable to type 'Formatter<ValueType, NameType> | undefined'.
                              Type 'Formatter<number, string> & ((value: number, name: string, item: TooltipPayloadEntry, index: number, payload: TooltipPayload) => [...] | ReactNode)' is not assignable to type 'Formatter<ValueType, NameType> | undefined'.
                                Type 'Formatter<number, string> & ((value: number, name: string, item: TooltipPayloadEntry, index: number, payload: TooltipPayload) => [...] | ReactNode)' is not assignable to type 'Formatter<ValueType, NameType>'.
                                  Types of parameters 'value' and 'value' are incompatible.
                                    Type 'ValueType | undefined' is not assignable to type 'number | undefined'.
                                      Type 'string' is not assignable to type 'number'.
src/pages/redemptions/RedemptionsPage.tsx(313,13): error TS2322: Type 'RedemptionRow[]' is not assignable to type 'LegacyFeatures[]'.
  Type 'RedemptionRow' is missing the following properties from type 'LegacyFeatures': aggregationFns, filterFns, sortFns, cellSelectionFeature, and 16 more.
src/pages/settings/SettingsPage.tsx(23,18): error TS18048: 'user.role' is possibly 'undefined'.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## unit-tests

```text
                [33mx2[39m=[32m"12.01"[39m
                [33my1[39m=[32m"16"[39m
                [33my2[39m=[32m"16"[39m
              [36m/>[39m
            [36m</svg>[39m
            [36m<div[39m
              [33mclass[39m=[32m"text-warning-foreground"[39m
            [36m>[39m
              [36m<p[39m
                [33mclass[39m=[32m"font-medium mb-0.5"[39m
              [36m>[39m
                [0mtopup.stripe.outcome_cancel_title[0m
              [36m</p>[39m
              [36m<p[39m
                [33mclass[39m=[32m"text-warning-foreground/80"[39m
              [36m>[39m
                [0mtopup.stripe.outcome_cancel[0m
              [36m</p>[39m
            [36m</div>[39m
          [36m</div>[39m
          [36m<div[39m
            [33mclass[39m=[32m"grid grid-cols-1 lg:grid-cols-5 gap-6"[39m
          [36m>[39m
            [36m<div[39m
              [33mclass[39m=[32m"rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"[39m
            [36m>[39m
              [36m<div[39m
                [33mclass[39m=[32m"p-6 h-full flex flex-col"[39m
              [36m>[39m
                [36m<div[39m
                  [33mclass[39m=[32m"flex items-start justify-between gap-2"[39m
                [36m>[39m
                  [36m<p[39m
                    [33mclass[39m=[32m"text-sm text-muted-foreground"[39m
                  [36m>[39m
                    [0mCurrent Balance[0m
                  [36m</p>[39m
                  [36m<button[39m
                    [33maria-label[39m=[32m"Refresh Balance"[39m
                    [33mclass[39m=[32m"inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"[39m
                    [33mdata-label[39m=[32m"Refresh Balance"[39m
                  [36m>[39m
                    [36m<svg[39m
                      [33maria-hidden[39m=[32m"true"[39m
                      [33mclass[39m=[32m"lucide lucide-refresh-cw h-4 w-4"[39m
                      [33mfill[39m=[32m"none"[39m
                      [33mheight[39m=[32m"24"[39m
                      [33mstroke[39m=[32m"currentColor"[39m
                      [33mstroke-linecap[39m=[32m"round"[39m
                      [33mstroke-linejoin[39m=[32m"round"[39m
                      [33mstroke-width[39m=[32m"2"[39m
                      [33mviewBox[39m=[32m"0 0 24 24"[39m
                      [33mwidth[39m=[32m"24"[39m
                      [33mxmlns[39m=[32m"http://www.w3.org/2000/svg"[39m
                    [36m>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M21 3v5h-5"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M8 16H3v5"[39m
                      [36m/>[39m
                    [36m</svg>[39m
                  [36m</button>[39m
                [36m</div>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"[39m
                [36m>[39m
                  [0m$0.00[0m
                [36m</p>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-1 text-xs text-muted-foreground tabular-nums"[39m
                [36m>[39m
                  [0m1,000 topup.tokens[0m
                [36m</p>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-auto pt-6 text-xs text-muted-foreground"[39m
                [36m>[39m
                  [0mtopup.balance.note[0m
                [36m</p>[39m
              [36m</div>[39m
            [36m</div>[39m
            [36m<div[39m
              [33mclass[39m=[32m"rounded-lg border bg-card text-card-foregro...
[90m [2m❯[22m waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:[2m163:27[22m[39m
[90m [2m❯[22m node_modules/@testing-library/dom/dist/query-helpers.js:[2m86:33[22m[39m
[36m [2m❯[22m src/pages/topup/__tests__/TopUpPage.amount.test.tsx:[2m118:18[22m[39m
    [90m116|[39m   [34mit[39m([32m'shows the cancel outcome from the query string'[39m[33m,[39m [35masync[39m () [33m=>[39m {
    [90m117|[39m     [34mrenderPage[39m([32m'/topup?stripe=cancel'[39m)[33m;[39m
    [90m118|[39m     [35mawait[39m screen[33m.[39m[34mfindByText[39m([36m/payment canceled/i[39m)[33m;[39m
    [90m   |[39m                  [31m^[39m
    [90m119|[39m     expect(screen.getByText(/you have not been charged/i)).toBeInTheDo…
    [90m120|[39m   })[33m;[39m

[31m[2m⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[10/24]⎯[22m[39m

[41m[1m FAIL [22m[49m src/pages/topup/__tests__/TopUpPage.amount.test.tsx[2m > [22mTopUpPage: Stripe checkout behavior[2m > [22mpolls fulfillment and refreshes balance and history after paid status
[31m[1mTestingLibraryElementError[22m[39m: Unable to find an element with the text: /^credits added$/i. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.

Ignored nodes: comments, script, style
[36m<body>[39m
  [36m<div>[39m
    [36m<div[39m
      [33mclass[39m=[32m"w-full mx-auto px-8 max-w-4xl"[39m
    [36m>[39m
      [36m<div[39m
        [33mclass[39m=[32m"mb-4 md:mb-6 space-y-2"[39m
      [36m>[39m
        [36m<div[39m
          [33mclass[39m=[32m"flex justify-between gap-4 flex-row items-center"[39m
        [36m>[39m
          [36m<div[39m
            [33mclass[39m=[32m"space-y-1"[39m
          [36m>[39m
            [36m<h1[39m
              [33mclass[39m=[32m"font-bold tracking-tight text-2xl md:text-3xl"[39m
            [36m>[39m
              [0mTop Up[0m
            [36m</h1>[39m
            [36m<p[39m
              [33mclass[39m=[32m"text-muted-foreground text-sm"[39m
            [36m>[39m
              [0mManage your account balance and redeem codes[0m
            [36m</p>[39m
          [36m</div>[39m
        [36m</div>[39m
      [36m</div>[39m
      [36m<div[39m
        [33mclass[39m=[32m"space-y-6"[39m
      [36m>[39m
        [36m<div[39m
          [33mclass[39m=[32m"space-y-6"[39m
        [36m>[39m
          [36m<div[39m
            [33mclass[39m=[32m"flex items-start gap-3 rounded-lg border border-success-border bg-success-muted px-4 py-3 text-sm"[39m
          [36m>[39m
            [36m<svg[39m
              [33maria-hidden[39m=[32m"true"[39m
              [33mclass[39m=[32m"lucide lucide-circle-check h-4 w-4 mt-0.5 text-success flex-shrink-0"[39m
              [33mfill[39m=[32m"none"[39m
              [33mheight[39m=[32m"24"[39m
              [33mstroke[39m=[32m"currentColor"[39m
              [33mstroke-linecap[39m=[32m"round"[39m
              [33mstroke-linejoin[39m=[32m"round"[39m
              [33mstroke-width[39m=[32m"2"[39m
              [33mviewBox[39m=[32m"0 0 24 24"[39m
              [33mwidth[39m=[32m"24"[39m
              [33mxmlns[39m=[32m"http://www.w3.org/2000/svg"[39m
            [36m>[39m
              [36m<circle[39m
                [33mcx[39m=[32m"12"[39m
                [33mcy[39m=[32m"12"[39m
                [33mr[39m=[32m"10"[39m
              [36m/>[39m
              [36m<path[39m
                [33md[39m=[32m"m9 12 2 2 4-4"[39m
              [36m/>[39m
            [36m</svg>[39m
            [36m<div[39m
              [33mclass[39m=[32m"text-success-foreground"[39m
            [36m>[39m
              [36m<p[39m
                [33mclass[39m=[32m"font-medium mb-0.5"[39m
              [36m>[39m
                [0mtopup.stripe.outcome_credited_title[0m
              [36m</p>[39m
              [36m<p[39m
                [33mclass[39m=[32m"text-success-foreground/80"[39m
              [36m>[39m
                [0mtopup.stripe.outcome_credited[0m
              [36m</p>[39m
            [36m</div>[39m
          [36m</div>[39m
          [36m<div[39m
            [33mclass[39m=[32m"grid grid-cols-1 lg:grid-cols-5 gap-6"[39m
          [36m>[39m
            [36m<div[39m
              [33mclass[39m=[32m"rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"[39m
            [36m>[39m
              [36m<div[39m
                [33mclass[39m=[32m"p-6 h-full flex flex-col"[39m
              [36m>[39m
                [36m<div[39m
                  [33mclass[39m=[32m"flex items-start justify-between gap-2"[39m
                [36m>[39m
                  [36m<p[39m
                    [33mclass[39m=[32m"text-sm text-muted-foreground"[39m
                  [36m>[39m
                    [0mCurrent Balance[0m
                  [36m</p>[39m
                  [36m<button[39m
                    [33maria-label[39m=[32m"Refresh Balance"[39m
                    [33mclass[39m=[32m"inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"[39m
                    [33mdata-label[39m=[32m"Refresh Balance"[39m
                  [36m>[39m
                    [36m<svg[39m
                      [33maria-hidden[39m=[32m"true"[39m
                      [33mclass[39m=[32m"lucide lucide-refresh-cw h-4 w-4"[39m
                      [33mfill[39m=[32m"none"[39m
                      [33mheight[39m=[32m"24"[39m
                      [33mstroke[39m=[32m"currentColor"[39m
                      [33mstroke-linecap[39m=[32m"round"[39m
                      [33mstroke-linejoin[39m=[32m"round"[39m
                      [33mstroke-width[39m=[32m"2"[39m
                      [33mviewBox[39m=[32m"0 0 24 24"[39m
                      [33mwidth[39m=[32m"24"[39m
                      [33mxmlns[39m=[32m"http://www.w3.org/2000/svg"[39m
                    [36m>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M21 3v5h-5"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M8 16H3v5"[39m
                      [36m/>[39m
                    [36m</svg>[39m
                  [36m</button>[39m
                [36m</div>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"[39m
                [36m>[39m
                  [0m$0.00[0m
                [36m</p>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-1 text-xs text-muted-foreground tabular-nums"[39m
                [36m>[39m
                  [0m1,000 topup.tokens[0m
                [36m</p>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-auto pt-6 text-xs text-muted-foreground"[39m
                [36m>[39m
                  [0mtopup.balance.note[0m
                [36m</p>[39m
              [36m</div>[39m
            [36m</div>[39m
            [36m<div[39m
              [33mclass[39m=[32m"rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"[39m
            [36m>[39m
              [36m<div[39m
                [33mclass[39m=[32m"flex flex-col space-y-1.5 p-6"[39m
              [36m>[39m
                [36m<div[39m
                  [33mclass[39m=[32m"flex items-center gap-2"[39m
                [36m>[39m
                  [36m<svg[39m
    ...

Ignored nodes: comments, script, style
[36m<body>[39m
  [36m<div>[39m
    [36m<div[39m
      [33mclass[39m=[32m"w-full mx-auto px-8 max-w-4xl"[39m
    [36m>[39m
      [36m<div[39m
        [33mclass[39m=[32m"mb-4 md:mb-6 space-y-2"[39m
      [36m>[39m
        [36m<div[39m
          [33mclass[39m=[32m"flex justify-between gap-4 flex-row items-center"[39m
        [36m>[39m
          [36m<div[39m
            [33mclass[39m=[32m"space-y-1"[39m
          [36m>[39m
            [36m<h1[39m
              [33mclass[39m=[32m"font-bold tracking-tight text-2xl md:text-3xl"[39m
            [36m>[39m
              [0mTop Up[0m
            [36m</h1>[39m
            [36m<p[39m
              [33mclass[39m=[32m"text-muted-foreground text-sm"[39m
            [36m>[39m
              [0mManage your account balance and redeem codes[0m
            [36m</p>[39m
          [36m</div>[39m
        [36m</div>[39m
      [36m</div>[39m
      [36m<div[39m
        [33mclass[39m=[32m"space-y-6"[39m
      [36m>[39m
        [36m<div[39m
          [33mclass[39m=[32m"space-y-6"[39m
        [36m>[39m
          [36m<div[39m
            [33mclass[39m=[32m"flex items-start gap-3 rounded-lg border border-success-border bg-success-muted px-4 py-3 text-sm"[39m
          [36m>[39m
            [36m<svg[39m
              [33maria-hidden[39m=[32m"true"[39m
              [33mclass[39m=[32m"lucide lucide-circle-check h-4 w-4 mt-0.5 text-success flex-shrink-0"[39m
              [33mfill[39m=[32m"none"[39m
              [33mheight[39m=[32m"24"[39m
              [33mstroke[39m=[32m"currentColor"[39m
              [33mstroke-linecap[39m=[32m"round"[39m
              [33mstroke-linejoin[39m=[32m"round"[39m
              [33mstroke-width[39m=[32m"2"[39m
              [33mviewBox[39m=[32m"0 0 24 24"[39m
              [33mwidth[39m=[32m"24"[39m
              [33mxmlns[39m=[32m"http://www.w3.org/2000/svg"[39m
            [36m>[39m
              [36m<circle[39m
                [33mcx[39m=[32m"12"[39m
                [33mcy[39m=[32m"12"[39m
                [33mr[39m=[32m"10"[39m
              [36m/>[39m
              [36m<path[39m
                [33md[39m=[32m"m9 12 2 2 4-4"[39m
              [36m/>[39m
            [36m</svg>[39m
            [36m<div[39m
              [33mclass[39m=[32m"text-success-foreground"[39m
            [36m>[39m
              [36m<p[39m
                [33mclass[39m=[32m"font-medium mb-0.5"[39m
              [36m>[39m
                [0mtopup.stripe.outcome_credited_title[0m
              [36m</p>[39m
              [36m<p[39m
                [33mclass[39m=[32m"text-success-foreground/80"[39m
              [36m>[39m
                [0mtopup.stripe.outcome_credited[0m
              [36m</p>[39m
            [36m</div>[39m
          [36m</div>[39m
          [36m<div[39m
            [33mclass[39m=[32m"grid grid-cols-1 lg:grid-cols-5 gap-6"[39m
          [36m>[39m
            [36m<div[39m
              [33mclass[39m=[32m"rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"[39m
            [36m>[39m
              [36m<div[39m
                [33mclass[39m=[32m"p-6 h-full flex flex-col"[39m
              [36m>[39m
                [36m<div[39m
                  [33mclass[39m=[32m"flex items-start justify-between gap-2"[39m
                [36m>[39m
                  [36m<p[39m
                    [33mclass[39m=[32m"text-sm text-muted-foreground"[39m
                  [36m>[39m
                    [0mCurrent Balance[0m
                  [36m</p>[39m
                  [36m<button[39m
                    [33maria-label[39m=[32m"Refresh Balance"[39m
                    [33mclass[39m=[32m"inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"[39m
                    [33mdata-label[39m=[32m"Refresh Balance"[39m
                  [36m>[39m
                    [36m<svg[39m
                      [33maria-hidden[39m=[32m"true"[39m
                      [33mclass[39m=[32m"lucide lucide-refresh-cw h-4 w-4"[39m
                      [33mfill[39m=[32m"none"[39m
                      [33mheight[39m=[32m"24"[39m
                      [33mstroke[39m=[32m"currentColor"[39m
                      [33mstroke-linecap[39m=[32m"round"[39m
                      [33mstroke-linejoin[39m=[32m"round"[39m
                      [33mstroke-width[39m=[32m"2"[39m
                      [33mviewBox[39m=[32m"0 0 24 24"[39m
                      [33mwidth[39m=[32m"24"[39m
                      [33mxmlns[39m=[32m"http://www.w3.org/2000/svg"[39m
                    [36m>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M21 3v5h-5"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"[39m
                      [36m/>[39m
                      [36m<path[39m
                        [33md[39m=[32m"M8 16H3v5"[39m
                      [36m/>[39m
                    [36m</svg>[39m
                  [36m</button>[39m
                [36m</div>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"[39m
                [36m>[39m
                  [0m$0.00[0m
                [36m</p>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-1 text-xs text-muted-foreground tabular-nums"[39m
                [36m>[39m
                  [0m1,000 topup.tokens[0m
                [36m</p>[39m
                [36m<p[39m
                  [33mclass[39m=[32m"mt-auto pt-6 text-xs text-muted-foreground"[39m
                [36m>[39m
                  [0mtopup.balance.note[0m
                [36m</p>[39m
              [36m</div>[39m
            [36m</div>[39m
            [36m<div[39m
              [33mclass[39m=[32m"rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"[39m
            [36m>[39m
              [36m<div[39m
                [33mclass[39m=[32m"flex flex-col space-y-1.5 p-6"[39m
              [36m>[39m
                [36m<div[39m
                  [33mclass[39m=[32m"flex items-center gap-2"[39m
                [36m>[39m
                  [36m<svg[39m
    ...
[90m [2m❯[22m waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:[2m163:27[22m[39m
[90m [2m❯[22m node_modules/@testing-library/dom/dist/query-helpers.js:[2m86:33[22m[39m
[36m [2m❯[22m src/pages/topup/__tests__/TopUpPage.amount.test.tsx:[2m128:18[22m[39m
    [90m126|[39m       expect(api.get).toHaveBeenCalledWith('/api/user/topup/stripe/ord…
    [90m127|[39m     )[33m;[39m
    [90m128|[39m     [35mawait[39m screen[33m.[39m[34mfindByText[39m([36m/^credits added$/i[39m)[33m;[39m
    [90m   |[39m                  [31m^[39m
    [90m129|[39m     [34mexpect[39m(
    [90m130|[39m       screen.getByText(/your balance has been updated from the server …

[31m[2m⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[11/24]⎯[22m[39m

[31m⎯⎯⎯⎯⎯⎯[39m[1m[41m Unhandled Errors [49m[22m[31m⎯⎯⎯⎯⎯⎯[39m
[31m[1m
Vitest caught 2 unhandled errors during the test run.
This might cause false positive tests. Resolve unhandled errors to make sure your tests are not affected.[22m[39m

[31m⎯⎯⎯⎯⎯[39m[1m[41m Uncaught Exception [49m[22m[31m⎯⎯⎯⎯⎯[39m
[31m[1mTypeError[22m: () => ({
	observe: vi.fn(),
	unobserve: vi.fn(),
	disconnect: __vit...<omitted>...}) is not a constructor[39m
[90m [2m❯[22m new Mock node_modules/@vitest/spy/dist/index.js:[2m309:27[22m[39m
[90m [2m❯[22m node_modules/@radix-ui/react-use-size/dist/index.mjs:[2m12:30[22m[39m
[90m [2m❯[22m Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:[2m25989:20[22m[39m
[90m [2m❯[22m runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:[2m874:13[22m[39m
[90m [2m❯[22m commitHookEffectListMount node_modules/react-dom/cjs/react-dom-client.development.js:[2m13249:29[22m[39m
[90m [2m❯[22m commitHookLayoutEffects node_modules/react-dom/cjs/react-dom-client.development.js:[2m13213:11[22m[39m
[90m [2m❯[22m commitLayoutEffectOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:[2m13951:13[22m[39m
[90m [2m❯[22m recursivelyTraverseLayoutEffects node_modules/react-dom/cjs/react-dom-client.development.js:[2m15159:11[22m[39m
[90m [2m❯[22m commitLayoutEffectOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:[2m14165:11[22m[39m
[90m [2m❯[22m recursivelyTraverseLayoutEffects node_modules/react-dom/cjs/react-dom-client.development.js:[2m15159:11[22m[39m

[31mThis error originated in "[1msrc/pages/users/__tests__/EditUserPage.test.tsx[22m" test file. It doesn't mean the error was thrown inside the file itself, but while it was running.[39m
[31mThe latest test that might've caused the error is "[1mrenders disable 2FA button when admin views another user and triggers API call on confirm[22m". It might mean one of the following:
- The error was thrown, while Vitest was running this test.
- If the error occurred after the test had been completed, this was the last documented test before it was thrown.[39m

[31m⎯⎯⎯⎯⎯[39m[1m[41m Uncaught Exception [49m[22m[31m⎯⎯⎯⎯⎯[39m
[31m[1mTypeError[22m: () => ({
	observe: vi.fn(),
	unobserve: vi.fn(),
	disconnect: __vit...<omitted>...}) is not a constructor[39m
[90m [2m❯[22m new Mock node_modules/@vitest/spy/dist/index.js:[2m309:27[22m[39m
[90m [2m❯[22m autoUpdate node_modules/@floating-ui/dom/dist/floating-ui.dom.mjs:[2m613:22[22m[39m
[90m [2m❯[22m Object.whileElementsMounted [as current] node_modules/@radix-ui/react-popper/dist/index.mjs:[2m131:25[22m[39m
[90m [2m❯[22m node_modules/@floating-ui/react-dom/dist/floating-ui.react-dom.mjs:[2m189:40[22m[39m
[90m [2m❯[22m Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:[2m25989:20[22m[39m
[90m [2m❯[22m runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:[2m874:13[22m[39m
[90m [2m❯[22m commitHookEffectListMount node_modules/react-dom/cjs/react-dom-client.development.js:[2m13249:29[22m[39m
[90m [2m❯[22m commitHookLayoutEffects node_modules/react-dom/cjs/react-dom-client.development.js:[2m13213:11[22m[39m
[90m [2m❯[22m commitLayoutEffectOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:[2m13951:13[22m[39m
[90m [2m❯[22m recursivelyTraverseLayoutEffects node_modules/react-dom/cjs/react-dom-client.development.js:[2m15159:11[22m[39m

[31mThis error originated in "[1msrc/components/ui/__tests__/timestamp.test.tsx[22m" test file. It doesn't mean the error was thrown inside the file itself, but while it was running.[39m
[31mThe latest test that might've caused the error is "[1mshows the UTC timestamp inside the tooltip[22m". It might mean one of the following:
- The error was thrown, while Vitest was running this test.
- If the error occurred after the test had been completed, this was the last documented test before it was thrown.[39m
[31m⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[39m


[2m Test Files [22m [1m[31m13 failed[39m[22m[2m | [22m[1m[32m51 passed[39m[22m[90m (64)[39m
[2m      Tests [22m [1m[31m22 failed[39m[22m[2m | [22m[1m[32m328 passed[39m[22m[2m | [22m[33m1 skipped[39m[90m (351)[39m
[2m     Errors [22m [1m[31m2 errors[39m[22m
[2m   Start at [22m 00:44:41
[2m   Duration [22m 46.41s[2m (transform 1.95s, setup 4.67s, import 15.65s, tests 31.65s, environment 31.80s)[22m


::error file=/Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx,title=src/components/layout/Header.test.tsx > Header logout UX > hides the logout action by default,line=82,column=22::TypeError: () => ({%0A	observe: vi.fn(),%0A	unobserve: vi.fn(),%0A	disconnect: __vit...<omitted>...}) is not a constructor%0A ❯ src/components/layout/HeaderNav.tsx:82:22%0A ❯ Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:25989:20%0A ❯ runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:874:13%0A ❯ commitHookEffectListMount node_modules/react-dom/cjs/react-dom-client.development.js:13249:29%0A ❯ commitHookPassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:13336:11%0A ❯ commitPassiveMountOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:15484:13%0A ❯ recursivelyTraversePassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:15439:11%0A ❯ commitPassiveMountOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:15718:11%0A ❯ recursivelyTraversePassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:15439:11%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx,title=src/components/layout/Header.test.tsx > Header logout UX > confirms logout through the desktop hamburger menu,line=82,column=22::TypeError: () => ({%0A	observe: vi.fn(),%0A	unobserve: vi.fn(),%0A	disconnect: __vit...<omitted>...}) is not a constructor%0A ❯ src/components/layout/HeaderNav.tsx:82:22%0A ❯ Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:25989:20%0A ❯ runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:874:13%0A ❯ commitHookEffectListMount node_modules/react-dom/cjs/react-dom-client.development.js:13249:29%0A ❯ commitHookPassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:13336:11%0A ❯ commitPassiveMountOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:15484:13%0A ❯ recursivelyTraversePassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:15439:11%0A ❯ commitPassiveMountOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:15718:11%0A ❯ recursivelyTraversePassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:15439:11%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/components/layout/HeaderNav.tsx,title=src/components/layout/__tests__/Header.topup.test.tsx > Header mobile overflow prevention > renders header with no horizontal overflow and truncates brand text,line=82,column=22::TypeError: () => ({%0A	observe: vi.fn(),%0A	unobserve: vi.fn(),%0A	disconnect: __vit...<omitted>...}) is not a constructor%0A ❯ src/components/layout/HeaderNav.tsx:82:22%0A ❯ Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:25989:20%0A ❯ runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:874:13%0A ❯ commitHookEffectListMount node_modules/react-dom/cjs/react-dom-client.development.js:13249:29%0A ❯ commitHookPassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:13336:11%0A ❯ commitPassiveMountOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:15484:13%0A ❯ recursivelyTraversePassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:15439:11%0A ❯ commitPassiveMountOnFiber node_modules/react-dom/cjs/react-dom-client.development.js:15718:11%0A ❯ recursivelyTraversePassiveMountEffects node_modules/react-dom/cjs/react-dom-client.development.js:15439:11%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/components/ui/__tests__/timestamp.test.tsx,title=src/components/ui/__tests__/timestamp.test.tsx > TimestampDisplay > shows the UTC timestamp inside the tooltip,line=26,column=42::TestingLibraryElementError: Unable to find an element with the text: 2024-10-13 18:40:00Z. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div />%0A</body>%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div />%0A</body>%0A ❯ waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:163:27%0A ❯ node_modules/@testing-library/dom/dist/query-helpers.js:86:33%0A ❯ src/components/ui/__tests__/timestamp.test.tsx:26:42%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/components/ui/data-table.tsx,title=src/pages/redemptions/__tests__/RedemptionsPage.test.tsx > RedemptionsPage action feedback > shows an error when delete returns success false,line=103,column=22::TypeError: getCoreRowModel is not a function%0A ❯ DataTable src/components/ui/data-table.tsx:103:22%0A ❯ Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:25904:20%0A ❯ renderWithHooks node_modules/react-dom/cjs/react-dom-client.development.js:7662:22%0A ❯ updateFunctionComponent node_modules/react-dom/cjs/react-dom-client.development.js:10166:19%0A ❯ beginWork node_modules/react-dom/cjs/react-dom-client.development.js:11778:18%0A ❯ runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:874:13%0A ❯ performUnitOfWork node_modules/react-dom/cjs/react-dom-client.development.js:17641:22%0A ❯ workLoopSync node_modules/react-dom/cjs/react-dom-client.development.js:17469:41%0A ❯ renderRootSync node_modules/react-dom/cjs/react-dom-client.development.js:17450:11%0A ❯ performWorkOnRoot node_modules/react-dom/cjs/react-dom-client.development.js:16583:35%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/components/ui/data-table.tsx,title=src/pages/redemptions/__tests__/RedemptionsPage.test.tsx > RedemptionsPage action feedback > shows an error when status update returns success false,line=103,column=22::TypeError: getCoreRowModel is not a function%0A ❯ DataTable src/components/ui/data-table.tsx:103:22%0A ❯ Object.react_stack_bottom_frame node_modules/react-dom/cjs/react-dom-client.development.js:25904:20%0A ❯ renderWithHooks node_modules/react-dom/cjs/react-dom-client.development.js:7662:22%0A ❯ updateFunctionComponent node_modules/react-dom/cjs/react-dom-client.development.js:10166:19%0A ❯ beginWork node_modules/react-dom/cjs/react-dom-client.development.js:11778:18%0A ❯ runWithFiberInDEV node_modules/react-dom/cjs/react-dom-client.development.js:874:13%0A ❯ performUnitOfWork node_modules/react-dom/cjs/react-dom-client.development.js:17641:22%0A ❯ workLoopSync node_modules/react-dom/cjs/react-dom-client.development.js:17469:41%0A ❯ renderRootSync node_modules/react-dom/cjs/react-dom-client.development.js:17450:11%0A ❯ performWorkOnRoot node_modules/react-dom/cjs/react-dom-client.development.js:16583:35%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/pages/users/__tests__/EditUserPage.test.tsx,title=src/pages/users/__tests__/EditUserPage.test.tsx > EditUserPage 2FA disable button > renders disable 2FA button when admin views another user and triggers API call on confirm,line=102,column=33::TestingLibraryElementError: Unable to find role="button" and name `/disable user 2fa/i`%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div />%0A</body>%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div />%0A</body>%0A ❯ waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:163:27%0A ❯ node_modules/@testing-library/dom/dist/query-helpers.js:86:33%0A ❯ src/pages/users/__tests__/EditUserPage.test.tsx:102:33%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/pages/topup/__tests__/TopUpPage.amount.test.tsx,title=src/pages/topup/__tests__/TopUpPage.amount.test.tsx > TopUpPage%3A Stripe checkout behavior > submits the exact two-decimal USD amount,line=96,column=33::TestingLibraryElementError: Unable to find a label with the text of: /amount \(usd\)/i%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"%0A            >%0A              <div%0A                class="flex flex-col space-y-1.5 p-6"%0A              >%0A                <div%0A                  class="flex items-center gap-2"%0A                >%0A                  <svg%0A                    aria-hidden="true"%0A                    class="lucide lucide-credit-card h-4 w-4 text-muted-foreground"%0A                    fill="none"%0A                    height="24"%0A                    stroke="currentColor"%0A                    stroke-linecap="round"%0A                    stroke-linejoin="round"%0A                    stroke-width="2"%0A                    viewBox="0 0 24 24"%0A                    width="24"%0A                    xmlns="http://www.w3.org/2000/svg"%0A                  >%0A                    <rect%0A                      height="14"%0A                      rx="2"%0A                      width="20"%0A                      x="2"%0A                      y="5"%0A                    />%0A                    <line%0A                      x1="2"%0A                      x2="22"%0A                      y1="10"%0A                      y2="10"%0A                    />%0A                  </svg>%0A                  <h3%0A                    class="text-lg font-semibold leading-none tracking-tight"%0A                  >%0A                    topup.stripe.title%0A                  </h3>%0A                </div>%0A                <p%0A                  class="text-sm text-muted-foreground"%0A                >%0A                  topup.stripe.descri...%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"%0A            >%0A              <div%0A                class="flex flex-col space-y-1.5 p-6"%0A              >%0A                <div%0A                  class="flex items-center gap-2"%0A                >%0A                  <svg%0A                    aria-hidden="true"%0A                    class="lucide lucide-credit-card h-4 w-4 text-muted-foreground"%0A                    fill="none"%0A                    height="24"%0A                    stroke="currentColor"%0A                    stroke-linecap="round"%0A                    stroke-linejoin="round"%0A                    stroke-width="2"%0A                    viewBox="0 0 24 24"%0A                    width="24"%0A                    xmlns="http://www.w3.org/2000/svg"%0A                  >%0A                    <rect%0A                      height="14"%0A                      rx="2"%0A                      width="20"%0A                      x="2"%0A                      y="5"%0A                    />%0A                    <line%0A                      x1="2"%0A                      x2="22"%0A                      y1="10"%0A                      y2="10"%0A                    />%0A                  </svg>%0A                  <h3%0A                    class="text-lg font-semibold leading-none tracking-tight"%0A                  >%0A                    topup.stripe.title%0A                  </h3>%0A                </div>%0A                <p%0A                  class="text-sm text-muted-foreground"%0A                >%0A                  topup.stripe.descri...%0A ❯ waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:163:27%0A ❯ node_modules/@testing-library/dom/dist/query-helpers.js:86:33%0A ❯ src/pages/topup/__tests__/TopUpPage.amount.test.tsx:96:33%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/pages/topup/__tests__/TopUpPage.amount.test.tsx,title=src/pages/topup/__tests__/TopUpPage.amount.test.tsx > TopUpPage%3A Stripe checkout behavior > blocks amounts below the server-advertised minimum,line=108,column=33::TestingLibraryElementError: Unable to find a label with the text of: /amount \(usd\)/i%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"%0A            >%0A              <div%0A                class="flex flex-col space-y-1.5 p-6"%0A              >%0A                <div%0A                  class="flex items-center gap-2"%0A                >%0A                  <svg%0A                    aria-hidden="true"%0A                    class="lucide lucide-credit-card h-4 w-4 text-muted-foreground"%0A                    fill="none"%0A                    height="24"%0A                    stroke="currentColor"%0A                    stroke-linecap="round"%0A                    stroke-linejoin="round"%0A                    stroke-width="2"%0A                    viewBox="0 0 24 24"%0A                    width="24"%0A                    xmlns="http://www.w3.org/2000/svg"%0A                  >%0A                    <rect%0A                      height="14"%0A                      rx="2"%0A                      width="20"%0A                      x="2"%0A                      y="5"%0A                    />%0A                    <line%0A                      x1="2"%0A                      x2="22"%0A                      y1="10"%0A                      y2="10"%0A                    />%0A                  </svg>%0A                  <h3%0A                    class="text-lg font-semibold leading-none tracking-tight"%0A                  >%0A                    topup.stripe.title%0A                  </h3>%0A                </div>%0A                <p%0A                  class="text-sm text-muted-foreground"%0A                >%0A                  topup.stripe.descri...%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"%0A            >%0A              <div%0A                class="flex flex-col space-y-1.5 p-6"%0A              >%0A                <div%0A                  class="flex items-center gap-2"%0A                >%0A                  <svg%0A                    aria-hidden="true"%0A                    class="lucide lucide-credit-card h-4 w-4 text-muted-foreground"%0A                    fill="none"%0A                    height="24"%0A                    stroke="currentColor"%0A                    stroke-linecap="round"%0A                    stroke-linejoin="round"%0A                    stroke-width="2"%0A                    viewBox="0 0 24 24"%0A                    width="24"%0A                    xmlns="http://www.w3.org/2000/svg"%0A                  >%0A                    <rect%0A                      height="14"%0A                      rx="2"%0A                      width="20"%0A                      x="2"%0A                      y="5"%0A                    />%0A                    <line%0A                      x1="2"%0A                      x2="22"%0A                      y1="10"%0A                      y2="10"%0A                    />%0A                  </svg>%0A                  <h3%0A                    class="text-lg font-semibold leading-none tracking-tight"%0A                  >%0A                    topup.stripe.title%0A                  </h3>%0A                </div>%0A                <p%0A                  class="text-sm text-muted-foreground"%0A                >%0A                  topup.stripe.descri...%0A ❯ waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:163:27%0A ❯ node_modules/@testing-library/dom/dist/query-helpers.js:86:33%0A ❯ src/pages/topup/__tests__/TopUpPage.amount.test.tsx:108:33%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/pages/topup/__tests__/TopUpPage.amount.test.tsx,title=src/pages/topup/__tests__/TopUpPage.amount.test.tsx > TopUpPage%3A Stripe checkout behavior > shows the cancel outcome from the query string,line=118,column=18::TestingLibraryElementError: Unable to find an element with the text: /payment canceled/i. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="flex items-start gap-3 rounded-lg border border-warning-border bg-warning-muted px-4 py-3 text-sm"%0A          >%0A            <svg%0A              aria-hidden="true"%0A              class="lucide lucide-circle-alert h-4 w-4 mt-0.5 text-warning flex-shrink-0"%0A              fill="none"%0A              height="24"%0A              stroke="currentColor"%0A              stroke-linecap="round"%0A              stroke-linejoin="round"%0A              stroke-width="2"%0A              viewBox="0 0 24 24"%0A              width="24"%0A              xmlns="http://www.w3.org/2000/svg"%0A            >%0A              <circle%0A                cx="12"%0A                cy="12"%0A                r="10"%0A              />%0A              <line%0A                x1="12"%0A                x2="12"%0A                y1="8"%0A                y2="12"%0A              />%0A              <line%0A                x1="12"%0A                x2="12.01"%0A                y1="16"%0A                y2="16"%0A              />%0A            </svg>%0A            <div%0A              class="text-warning-foreground"%0A            >%0A              <p%0A                class="font-medium mb-0.5"%0A              >%0A                topup.stripe.outcome_cancel_title%0A              </p>%0A              <p%0A                class="text-warning-foreground/80"%0A              >%0A                topup.stripe.outcome_cancel%0A              </p>%0A            </div>%0A          </div>%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foregro...%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="flex items-start gap-3 rounded-lg border border-warning-border bg-warning-muted px-4 py-3 text-sm"%0A          >%0A            <svg%0A              aria-hidden="true"%0A              class="lucide lucide-circle-alert h-4 w-4 mt-0.5 text-warning flex-shrink-0"%0A              fill="none"%0A              height="24"%0A              stroke="currentColor"%0A              stroke-linecap="round"%0A              stroke-linejoin="round"%0A              stroke-width="2"%0A              viewBox="0 0 24 24"%0A              width="24"%0A              xmlns="http://www.w3.org/2000/svg"%0A            >%0A              <circle%0A                cx="12"%0A                cy="12"%0A                r="10"%0A              />%0A              <line%0A                x1="12"%0A                x2="12"%0A                y1="8"%0A                y2="12"%0A              />%0A              <line%0A                x1="12"%0A                x2="12.01"%0A                y1="16"%0A                y2="16"%0A              />%0A            </svg>%0A            <div%0A              class="text-warning-foreground"%0A            >%0A              <p%0A                class="font-medium mb-0.5"%0A              >%0A                topup.stripe.outcome_cancel_title%0A              </p>%0A              <p%0A                class="text-warning-foreground/80"%0A              >%0A                topup.stripe.outcome_cancel%0A              </p>%0A            </div>%0A          </div>%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foregro...%0A ❯ waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:163:27%0A ❯ node_modules/@testing-library/dom/dist/query-helpers.js:86:33%0A ❯ src/pages/topup/__tests__/TopUpPage.amount.test.tsx:118:18%0A%0A

::error file=/Users/runner/work/one-api/one-api/web/modern/src/pages/topup/__tests__/TopUpPage.amount.test.tsx,title=src/pages/topup/__tests__/TopUpPage.amount.test.tsx > TopUpPage%3A Stripe checkout behavior > polls fulfillment and refreshes balance and history after paid status,line=128,column=18::TestingLibraryElementError: Unable to find an element with the text: /^credits added$/i. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="flex items-start gap-3 rounded-lg border border-success-border bg-success-muted px-4 py-3 text-sm"%0A          >%0A            <svg%0A              aria-hidden="true"%0A              class="lucide lucide-circle-check h-4 w-4 mt-0.5 text-success flex-shrink-0"%0A              fill="none"%0A              height="24"%0A              stroke="currentColor"%0A              stroke-linecap="round"%0A              stroke-linejoin="round"%0A              stroke-width="2"%0A              viewBox="0 0 24 24"%0A              width="24"%0A              xmlns="http://www.w3.org/2000/svg"%0A            >%0A              <circle%0A                cx="12"%0A                cy="12"%0A                r="10"%0A              />%0A              <path%0A                d="m9 12 2 2 4-4"%0A              />%0A            </svg>%0A            <div%0A              class="text-success-foreground"%0A            >%0A              <p%0A                class="font-medium mb-0.5"%0A              >%0A                topup.stripe.outcome_credited_title%0A              </p>%0A              <p%0A                class="text-success-foreground/80"%0A              >%0A                topup.stripe.outcome_credited%0A              </p>%0A            </div>%0A          </div>%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"%0A            >%0A              <div%0A                class="flex flex-col space-y-1.5 p-6"%0A              >%0A                <div%0A                  class="flex items-center gap-2"%0A                >%0A                  <svg%0A    ...%0A%0AIgnored nodes: comments, script, style%0A<body>%0A  <div>%0A    <div%0A      class="w-full mx-auto px-8 max-w-4xl"%0A    >%0A      <div%0A        class="mb-4 md:mb-6 space-y-2"%0A      >%0A        <div%0A          class="flex justify-between gap-4 flex-row items-center"%0A        >%0A          <div%0A            class="space-y-1"%0A          >%0A            <h1%0A              class="font-bold tracking-tight text-2xl md:text-3xl"%0A            >%0A              Top Up%0A            </h1>%0A            <p%0A              class="text-muted-foreground text-sm"%0A            >%0A              Manage your account balance and redeem codes%0A            </p>%0A          </div>%0A        </div>%0A      </div>%0A      <div%0A        class="space-y-6"%0A      >%0A        <div%0A          class="space-y-6"%0A        >%0A          <div%0A            class="flex items-start gap-3 rounded-lg border border-success-border bg-success-muted px-4 py-3 text-sm"%0A          >%0A            <svg%0A              aria-hidden="true"%0A              class="lucide lucide-circle-check h-4 w-4 mt-0.5 text-success flex-shrink-0"%0A              fill="none"%0A              height="24"%0A              stroke="currentColor"%0A              stroke-linecap="round"%0A              stroke-linejoin="round"%0A              stroke-width="2"%0A              viewBox="0 0 24 24"%0A              width="24"%0A              xmlns="http://www.w3.org/2000/svg"%0A            >%0A              <circle%0A                cx="12"%0A                cy="12"%0A                r="10"%0A              />%0A              <path%0A                d="m9 12 2 2 4-4"%0A              />%0A            </svg>%0A            <div%0A              class="text-success-foreground"%0A            >%0A              <p%0A                class="font-medium mb-0.5"%0A              >%0A                topup.stripe.outcome_credited_title%0A              </p>%0A              <p%0A                class="text-success-foreground/80"%0A              >%0A                topup.stripe.outcome_credited%0A              </p>%0A            </div>%0A          </div>%0A          <div%0A            class="grid grid-cols-1 lg:grid-cols-5 gap-6"%0A          >%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-2"%0A            >%0A              <div%0A                class="p-6 h-full flex flex-col"%0A              >%0A                <div%0A                  class="flex items-start justify-between gap-2"%0A                >%0A                  <p%0A                    class="text-sm text-muted-foreground"%0A                  >%0A                    Current Balance%0A                  </p>%0A                  <button%0A                    aria-label="Refresh Balance"%0A                    class="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none bg-transparent hover:bg-accent px-3 -mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"%0A                    data-label="Refresh Balance"%0A                  >%0A                    <svg%0A                      aria-hidden="true"%0A                      class="lucide lucide-refresh-cw h-4 w-4"%0A                      fill="none"%0A                      height="24"%0A                      stroke="currentColor"%0A                      stroke-linecap="round"%0A                      stroke-linejoin="round"%0A                      stroke-width="2"%0A                      viewBox="0 0 24 24"%0A                      width="24"%0A                      xmlns="http://www.w3.org/2000/svg"%0A                    >%0A                      <path%0A                        d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"%0A                      />%0A                      <path%0A                        d="M21 3v5h-5"%0A                      />%0A                      <path%0A                        d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"%0A                      />%0A                      <path%0A                        d="M8 16H3v5"%0A                      />%0A                    </svg>%0A                  </button>%0A                </div>%0A                <p%0A                  class="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground"%0A                >%0A                  $0.00%0A                </p>%0A                <p%0A                  class="mt-1 text-xs text-muted-foreground tabular-nums"%0A                >%0A                  1,000 topup.tokens%0A                </p>%0A                <p%0A                  class="mt-auto pt-6 text-xs text-muted-foreground"%0A                >%0A                  topup.balance.note%0A                </p>%0A              </div>%0A            </div>%0A            <div%0A              class="rounded-lg border bg-card text-card-foreground shadow-sm lg:col-span-3"%0A            >%0A              <div%0A                class="flex flex-col space-y-1.5 p-6"%0A              >%0A                <div%0A                  class="flex items-center gap-2"%0A                >%0A                  <svg%0A    ...%0A ❯ waitForWrapper node_modules/@testing-library/dom/dist/wait-for.js:163:27%0A ❯ node_modules/@testing-library/dom/dist/query-helpers.js:86:33%0A ❯ src/pages/topup/__tests__/TopUpPage.amount.test.tsx:128:18%0A%0A
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```
