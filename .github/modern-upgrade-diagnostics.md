# Modern latest-stable compatibility diagnostics

- Resolver base: latest `main` at staging reset
- Node: `24.x`
- Yarn: `1.22.22`

## Check status

```text
current=0
type-check=0
lint=0
i18n=0
unit-tests=1
production-build=1
frozen-lockfile=0
```

## Vitest summary

- suites: 152 total, 2 failed
- tests: 351 total, 1 failed

```text
TopUpPage: Stripe checkout behavior > blocks amounts below the server-advertised minimum
  Error: Unable to find an element with the text: /minimum is \$5/i. This could be because the text is broken up by multiple elements. In this case, you can provide a function for your text matcher to make your matcher more flexible.
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
```

## production-build

```text
yarn run v1.22.22
$ tsc -b && vite build --mode production --emptyOutDir
[36mvite v8.2.1 [32mbuilding client environment for production...[36m[39m
[33mWarning: Invalid input options (1 issue found)
- For the "treeshake.preset". Invalid key: Expected never but received "preset". [0m
[33mWarning: Invalid output options (1 issue found)
- For the "manualChunks". Invalid type: Expected Function but received Object. [0m
transforming...
Error: Cannot apply unknown utility class `mobile-table`
    at onInvalidCandidate (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:21:1879)
    at Oe (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:16:39470)
    at /Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:21:711
    at ai (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:5:1775)
    at O (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:5:1377)
    at Pe (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:21:172)
    at On (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:38:294)
    at async _n (/Users/runner/work/one-api/one-api/web/modern/node_modules/tailwindcss/dist/lib.js:38:631)
    at async an (/Users/runner/work/one-api/one-api/web/modern/node_modules/@tailwindcss/node/dist/index.js:10:3415)
    at async E (/Users/runner/work/one-api/one-api/web/modern/node_modules/@tailwindcss/postcss/dist/index.js:10:3996)
✓ 3573 modules transformed.
[31m✗[39m Build failed in 534ms
[31merror during build:
[31mBuild failed with 1 error:

[plugin vite:css] /Users/runner/work/one-api/one-api/web/modern/src/index.css:1:0
CssSyntaxError: [postcss] tailwindcss: /Users/runner/work/one-api/one-api/web/modern/src/index.css:1:1: Cannot apply unknown utility class `mobile-table`
    at Input.error (/Users/runner/work/one-api/one-api/web/modern/node_modules/postcss/lib/input.js:135:16)
    at Root.error (/Users/runner/work/one-api/one-api/web/modern/node_modules/postcss/lib/node.js:166:32)
    at Object.Once (/Users/runner/work/one-api/one-api/web/modern/node_modules/@tailwindcss/postcss/dist/index.js:10:7013)
    at async LazyResult.runAsync (/Users/runner/work/one-api/one-api/web/modern/node_modules/postcss/lib/lazy-result.js:299:11)
    at async runPostCSS (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:23164:19)
    at async compilePostCSS (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:23148:6)
    at async compileCSS (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:23078:26)
    at async TransformPluginContextImpl.handler (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:22556:47)
    at async plugin (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/rolldown/dist/shared/bindingify-input-options-COG1sboO.mjs:1537:16)
    at async plugin.<computed> (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/rolldown/dist/shared/bindingify-input-options-COG1sboO.mjs:2125:12)[31m
    at aggregateBindingErrorsIntoJsError (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/rolldown/dist/shared/error-NcRuXAVN.mjs:48:18)
    at unwrapBindingResult (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/rolldown/dist/shared/error-NcRuXAVN.mjs:18:128)
    at #build (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/rolldown/dist/shared/rolldown-vcii7mEa.mjs:132:34)
    at async buildEnvironment (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:33730:66)
    at async Object.build (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:34150:19)
    at async Object.buildApp (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/chunks/node.js:34147:153)
    at async CAC.<anonymous> (file:///Users/runner/work/one-api/one-api/web/modern/node_modules/vite/dist/node/cli.js:776:3) {
  errors: [Getter/Setter]
}[39m
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
