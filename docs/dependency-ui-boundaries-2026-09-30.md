# Dependency upgrade: UI error ownership and build cleanup

This follow-up is part of PR #437 and complements [the dependency migration acceptance](dependency-refresh-2026-09-30.md). It closes issues found while checking the callers of the corrected Axios rejection interceptor, rather than limiting validation to the initially reported token loader.

## Error ownership

Request helpers still reject with the original error. An awaiting internal caller must see that rejection; treating an unsuccessful request as a successful `undefined` response would restore the original defect.

At a terminal boundary, an ignored promise needs an error handler. Detached local asynchronous calls and promise chains now end with the existing notification helper. DOM-style UI events and Formik submit callbacks handle their final rejection while preserving successful arguments and results. Errors already displayed by the interceptor are deduplicated by object identity. Catch blocks pass the error object rather than just its message so this identity is retained.

The change deliberately does not wrap direct `onOk`/`onConfirm` callbacks whose promise contract is owned by a UI component. A regression protects that distinction. There is no global unhandled-rejection suppression or blanket error swallowing.

The committed source inventory regression checks both legacy trees for unowned detached local async calls and the affected event attributes. Behavioral tests evaluate the actual JSX callback expression and actual request handler code; they do not test a replacement implementation.

## Failed deletion must retain the row

Air's channel, redemption and user tables had the same failure pattern: a mutation returned normally after a `success: false` response, then a success-only `.then(...)` callback removed the visible row anyway.

Those mutation functions now reject a failed business operation. Their existing success callbacks therefore run only after a confirmed successful response, and the terminal handler displays the failure. Regression cases cover business failure, transport failure with original error identity, and success for each of the three real mutation/callback pairs. The prior token mutation regressions remain in place.

## Browser and negative-control evidence

[The UI-boundary acceptance run](https://github.com/Laisky/one-api/actions/runs/36775604472) passed all jobs, including the explicit old-code negative controls, lint, full dependency audits, production builds and both production/development browser modes. Air ran 54 test executions and Berry ran 42.

The original `bd55f65` event callback rejects without handling the injected failure. Its channel/redemption/user deletion callbacks also remove rows after a failed business operation. The same tests fail on that revision and pass with the fixes.

The browser suite additionally opens the real Air user-creation form, injects a failed POST, verifies that the input and form remain usable, then clicks submit again and verifies success. It repeats this against both built production assets and the Vite development proxy, with no unexpected browser page errors. The password-reset and token-list failure/retry scenarios remain required for both themes.

## Build cleanup is still a required safety contract

The old Go test checked for the literal CRA shell fragment `rm -rf ../build/<theme>`. Vite performs that cleanup through `build.emptyOutDir`, so retaining the old string assertion would test an obsolete implementation rather than the safety property.

`TestLegacyThemesCleanupBuildOutput` now evaluates the actual shared configuration through Node's built-in module APIs, without installing or importing Vite plugins. It requires the selected theme's exact output path and `emptyOutDir: true`, and checks that all three build commands retain lint and use Vite. It never skips the check. Node.js is required for this repository-level frontend contract test; use Node 24 as in CI.

The entire Go race/database matrix and final PR checks still determine acceptance of the latest commit. Historical validation evidence is not a substitute for the current check results.

## Reproduce

```sh
(cd web/air && yarn install --frozen-lockfile && yarn test && yarn build:prod)
(cd web/berry && yarn install --frozen-lockfile && yarn test && yarn build:prod)
python3 .github/scripts/legacy-browser.py air
python3 .github/scripts/legacy-browser.py berry
go test -race -count=1 -run '^TestLegacyThemesCleanupBuildOutput$' ./test
```

The historical command below must fail on the intended event/mutation assertions:

```sh
LEGACY_THEME=air \
LEGACY_UI_BASELINE=bd55f65a0bec27cbe01f67cdae06429cb3c30fa1 \
node --test --test-name-pattern='UI event failure|business failure' \
  .github/scripts/legacy-ui-boundaries.test.cjs
```
