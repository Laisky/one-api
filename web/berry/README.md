# One API Berry frontend

This frontend is based on [Berry Free React Admin Template](https://github.com/codedthemes/berry-free-react-admin-template). It uses React 18, Vite and Yarn Classic. Use Node 24, matching CI.

## Development and acceptance

```sh
yarn install --frozen-lockfile --non-interactive
yarn dev                 # http://127.0.0.1:3003
yarn test                # Transport, security, lifecycle and build regressions
yarn build               # Lint and build into ../build/berry
yarn build:prod          # Production build with the existing timestamp version
yarn build:dev           # Development-mode build with source maps
```

Development `/api` requests use the `proxy` in `package.json`; override it with `PROXY_TARGET=http://127.0.0.1:3000`. `HOST` and `PORT` override the loopback address and theme port. `REACT_APP_SERVER` selects another API origin at build time. `REACT_APP_VERSION` supplies a version for `yarn build`; `build:prod` and `dev:backend` retain timestamp stamping.

The HTML entry is `index.html`; static assets remain in `public`. Shared configuration in `../legacy/vite-config.mjs` preserves absolute source imports, SVG React components and the independent output directory. The retired CRA service worker is no longer registered; cleanup targets only the matching old worker, not other applications.

CI requires zero-advisory full-tree audits, lint/build checks and real-browser failure/retry acceptance against both production assets and the development proxy. See [dependency quality acceptance](../../docs/dependency-refresh-2026-09-30.md) for the commands and evidence.

## Adding a channel

Add the channel to `CHANNEL_OPTIONS` in `src/constants/ChannelConstants.js`:

```js
export const CHANNEL_OPTIONS = {
  1: {
    key: 1, // Channel identifier.
    text: 'OpenAI',
    value: 1,
    color: 'primary',
  },
};
```

Add a matching `typeConfig` entry in `src/views/Channel/type/Config.js` when the channel requires additional configuration:

```js
const typeConfig = {
  3: {
    inputLabel: {
      base_url: 'AZURE_OPENAI_ENDPOINT',
      other: 'Default API version',
    },
    prompt: {
      base_url: 'Enter AZURE_OPENAI_ENDPOINT',
      // The other input is shown only when this property is present.
      other: 'Enter a default API version, such as 2024-03-01-preview',
    },
    modelGroup: 'openai', // Selects the models used by the autofill button.
  },
};
```

## Acknowledgments and license

This frontend incorporates Berry Free React Admin Template and minimal-ui-kit. The incorporated code follows the MIT license.
