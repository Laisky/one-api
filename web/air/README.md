# One API Air frontend

Air uses React 18, Vite and Yarn Classic. Use Node 24, matching CI.

```sh
yarn install --frozen-lockfile --non-interactive
yarn dev                 # http://127.0.0.1:3002
yarn test                # Transport, security, lifecycle and build regressions
yarn build               # Lint and build into ../build/air
yarn build:prod          # Production build with the existing timestamp version
yarn build:dev           # Development-mode build with source maps
```

Development `/api` requests use the `proxy` in `package.json`. Override it with `PROXY_TARGET=http://127.0.0.1:3000`. `HOST` and `PORT` override the loopback address and theme port. Do not expose a development server publicly without an access-control boundary.

`REACT_APP_SERVER` selects a different API origin at build time. `REACT_APP_VERSION` supplies a build version for `yarn build`; the explicit `build:prod` and `dev:backend` commands retain timestamp stamping. Only these intended public constants are exposed, not arbitrary server environment variables.

Vite entry HTML is `index.html`; static assets remain in `public`. Shared configuration lives in `../legacy/vite-config.mjs`. Existing source aliases, JSX in `.js`, the independent output directory and API proxy are covered by tests. Both production bundles and the development proxy receive real-browser failure/retry acceptance in CI.

See [dependency quality acceptance](../../docs/dependency-refresh-2026-09-30.md) for the full-tree audit, migration decisions and reproducible browser tests.

## References

- https://github.com/OIerDb-ng/OIerDb
- https://github.com/cornflourblue/react-hooks-redux-registration-login-example
