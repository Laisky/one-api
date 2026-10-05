# Deployment-bound discovery metadata

Discovery URLs are derived from the administrator's `ServerAddress` setting.
Configure the public HTTP or HTTPS origin, including a port when required, with
no user information, path prefix, query, or fragment. Request `Host`, `Forwarded`,
and `X-Forwarded-*` headers cannot override this setting.

The router captures the configured origin once per request. It binds Link
headers, embedded documentation and metadata, root and well-known aliases,
public MCP replies, the MCP Apps view and `/ask` responses to that origin.
Origin-bearing responses use `Cache-Control: no-store` so a changed setting does
not leave a cache advertising the previous credential destination. Invalid
configuration suppresses discovery links and returns HTTP 503 for origin-bearing
metadata. Ordinary frontend assets without origin metadata remain available.

The hosted addresses in redistributable public files are templates. Serve these
files through One API's router; serving the raw public directory from a separate
static host does not execute origin binding. A separately hosted frontend must
route discovery endpoints through the backend or perform equivalent explicit
deployment configuration during publication.

## Regression evidence

The retained `TestSecurityDiscoveryOriginHTTP` uses the shipped manifest and two
local HTTP deployments. On the unchanged baseline, a deliberately
metadata-following client delivered its synthetic credential to the separate
hosted-origin fixture. Both cases fail before the fix and pass afterward. This
demonstrates conditional client behavior; it does not claim universal client
credential forwarding or a production disclosure.

`TestDiscoveryShippedRoutes` exercises the production router against every
origin-bearing public asset and generated discovery responses, including
extensionless OAuth aliases, spoofed request headers, and two successive
administrator settings. `TestDiscoveryOriginValidation` covers malformed and
unsafe configured origins, loopback HTTP, HTTPS, and IPv6.

To exercise the actual compiled Modern assets after the required build:

```sh
make build-frontend-modern
ONEAPI_DISCOVERY_BUILD_DIR=../web/build/modern go test -race ./router -run 'TestDiscovery|TestSecurityDiscovery'
```

The directory is resolved from the router test's working directory. The default
test mode reads the shipped public assets without requiring a frontend build.
