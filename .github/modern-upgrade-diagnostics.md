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

- suites: 152 total, 2 failed
- tests: 351 total, 2 failed

```text
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
```

## production-build

```text
yarn run v1.22.22
$ tsc -b && vite build --mode production --emptyOutDir
src/pages/channels/schemas.ts(59,61): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Type 'string' is not assignable to type '"chat_completion" | "response"'.
error Command failed with exit code 1.
info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command.
```

## type-check

```text
yarn run v1.22.22
$ tsc --noEmit
src/pages/channels/schemas.ts(59,61): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Type 'string' is not assignable to type '"chat_completion" | "response"'.
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
