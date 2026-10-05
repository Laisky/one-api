# Kubernetes Deployment

<p align="center">
   <img src="https://kubernetes.io/images/kubernetes.png" alt="sailing-with-k8s" width="80">
</p>

This section provides comprehensive instructions for deploying One API on Kubernetes with various configurations.

## Prerequisites

- A supported Kubernetes cluster compatible with the gateway release; see the version matrix in the gateway section below
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) configured to communicate with your cluster
- [`helm`](https://helm.sh/docs/intro/install/) with OCI support for the pinned gateway installation

## Basic Deployment

### Namespace

First, create a dedicated namespace for One API:

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: one-api
  labels:
    name: one-api
```

```bash
kubectl apply -f namespace.yaml
```

### ConfigMap

Create a ConfigMap for One API configuration:

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: one-api-config
  namespace: one-api
data:
  # Basic configuration
  SESSION_SECRET: 'your-session-secret-here'
  DEBUG: 'false'
  DEBUG_SQL: 'false'
  # Rate limiting
  GLOBAL_API_RATE_LIMIT: '1000'
  GLOBAL_WEB_RATE_LIMIT: '1000'
  GLOBAL_RELAY_RATE_LIMIT: '1000'
  GLOBAL_CHANNEL_RATE_LIMIT: '1'
  # Token settings
  DEFAULT_MAX_TOKEN: '2048'
  MAX_INLINE_IMAGE_SIZE_MB: '30'
  MAX_ITEMS_PER_PAGE: '10'

  # Channel settings
  CHANNEL_SUSPEND_SECONDS_FOR_429: '60'
  OPENROUTER_PROVIDER_SORT: 'throughput'

  # Usage enforcement
  ENFORCE_INCLUDE_USAGE: 'true'
```

```bash
kubectl apply -f configmap.yaml
```

### Deployment

Create the main One API deployment:

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: one-api
  namespace: one-api
  labels:
    app: one-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: one-api
  template:
    metadata:
      labels:
        app: one-api
    spec:
      containers:
        - name: one-api
          image: ppcelery/one-api:latest
          ports:
            - containerPort: 3000
              name: http
          envFrom:
            - configMapRef:
                name: one-api-config
            - secretRef:
                name: one-api-secrets
                optional: true
          env:
            - name: SQL_DSN
              valueFrom:
                secretKeyRef:
                  name: one-api-database
                  key: dsn
            - name: REDIS_CONN_STRING
              valueFrom:
                secretKeyRef:
                  name: one-api-redis
                  key: connection-string
                  optional: true
          volumeMounts:
            - name: data
              mountPath: /data
          resources:
            requests:
              memory: '256Mi'
              cpu: '250m'
            limits:
              memory: '1Gi'
              cpu: '1000m'
          livenessProbe:
            httpGet:
              path: /api/status
              port: 3000
            initialDelaySeconds: 30
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /api/status
              port: 3000
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: one-api-service
  namespace: one-api
  labels:
    app: one-api
spec:
  selector:
    app: one-api
  ports:
    - port: 80
      targetPort: 3000
      protocol: TCP
      name: http
  type: ClusterIP
```

```bash
kubectl apply -f deployment.yaml
```

### Database Setup

One API supports multiple database backends. Here are examples for PostgreSQL and MySQL:

### PostgreSQL Setup

```yaml
# postgresql.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  namespace: one-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
        - name: postgresql
          image: postgres:15
          env:
            - name: POSTGRES_DB
              value: 'oneapi'
            - name: POSTGRES_USER
              valueFrom:
                secretKeyRef:
                  name: postgresql-secret
                  key: username
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: postgresql-secret
                  key: password
            - name: PGDATA
              value: /var/lib/postgresql/data/pgdata
          ports:
            - containerPort: 5432
          volumeMounts:
            - name: postgresql-storage
              mountPath: /var/lib/postgresql/data
          resources:
            requests:
              memory: '256Mi'
              cpu: '250m'
            limits:
              memory: '1Gi'
              cpu: '500m'
      volumes:
        - name: postgresql-storage
          persistentVolumeClaim:
            claimName: postgresql-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgresql-pvc
  namespace: one-api
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql-service
  namespace: one-api
spec:
  selector:
    app: postgresql
  ports:
    - port: 5432
      targetPort: 5432
---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
  namespace: one-api
type: Opaque
data:
  username: b25lYXBp # oneapi (base64)
  password: cGFzc3dvcmQ= # password (base64) - Change this!
---
apiVersion: v1
kind: Secret
metadata:
  name: one-api-database
  namespace: one-api
type: Opaque
data:
  dsn: cG9zdGdyZXM6Ly9vbmVhcGk6cGFzc3dvcmRAcG9zdGdyZXNxbC1zZXJ2aWNlOjU0MzIvb25lYXBpP3NzbG1vZGU9ZGlzYWJsZQ==
  # postgres://oneapi:password@postgresql-service:5432/oneapi?sslmode=disable (base64)
```

```bash
kubectl apply -f postgresql.yaml
```

> [!NOTE] > **PostgreSQL Version**: The example above uses PostgreSQL version `15`. Check the [PostgreSQL Docker Hub page](https://hub.docker.com/_/postgres) for available versions and update accordingly. Consider using specific minor versions like `postgres:15.8` for production environments to ensure consistency.

### MySQL Setup

```yaml
# mysql.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
  namespace: one-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
        - name: mysql
          image: mysql:8.0
          env:
            - name: MYSQL_DATABASE
              value: 'oneapi'
            - name: MYSQL_USER
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: username
            - name: MYSQL_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: password
            - name: MYSQL_ROOT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: root-password
          ports:
            - containerPort: 3306
          volumeMounts:
            - name: mysql-storage
              mountPath: /var/lib/mysql
          resources:
            requests:
              memory: '256Mi'
              cpu: '250m'
            limits:
              memory: '1Gi'
              cpu: '500m'
      volumes:
        - name: mysql-storage
          persistentVolumeClaim:
            claimName: mysql-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-pvc
  namespace: one-api
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: mysql-service
  namespace: one-api
spec:
  selector:
    app: mysql
  ports:
    - port: 3306
      targetPort: 3306
---
apiVersion: v1
kind: Secret
metadata:
  name: mysql-secret
  namespace: one-api
type: Opaque
data:
  username: b25lYXBp # oneapi (base64)
  password: cGFzc3dvcmQ= # password (base64) - Change this!
  root-password: cm9vdHBhc3N3b3Jk # rootpassword (base64) - Change this!
---
apiVersion: v1
kind: Secret
metadata:
  name: one-api-database
  namespace: one-api
type: Opaque
data:
  dsn: b25lYXBpOnBhc3N3b3JkQG15c3FsLXNlcnZpY2U6MzMwNi9vbmVhcGk/Y2hhcnNldD11dGY4bWI0JnBhcnNlVGltZT1UcnVlJmxvYz1Mb2NhbA==
  # oneapi:password@mysql-service:3306/oneapi?charset=utf8mb4&parseTime=True&loc=Local (base64)
```

```bash
kubectl apply -f mysql.yaml
```

> [!NOTE] > **MySQL Version**: The example above uses MySQL version `8.0`. Check the [MySQL Docker Hub page](https://hub.docker.com/_/mysql) for available versions and update accordingly. Consider using specific minor versions like `mysql:8.0.39` for production environments to ensure consistency.

### Redis Setup

For caching and improved performance, deploy Redis:

```yaml
# redis.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: one-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: redis:7-alpine
          ports:
            - containerPort: 6379
          args:
            - redis-server
            - --appendonly
            - 'yes'
            - --requirepass
            - '$(REDIS_PASSWORD)'
          env:
            - name: REDIS_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: redis-secret
                  key: password
          volumeMounts:
            - name: redis-storage
              mountPath: /data
          resources:
            requests:
              memory: '64Mi'
              cpu: '100m'
            limits:
              memory: '256Mi'
              cpu: '200m'
      volumes:
        - name: redis-storage
          persistentVolumeClaim:
            claimName: redis-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: redis-pvc
  namespace: one-api
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: one-api
spec:
  selector:
    app: redis
  ports:
    - port: 6379
      targetPort: 6379
---
apiVersion: v1
kind: Secret
metadata:
  name: redis-secret
  namespace: one-api
type: Opaque
data:
  password: cmVkaXNwYXNzd29yZA== # redispassword (base64) - Change this!
---
apiVersion: v1
kind: Secret
metadata:
  name: one-api-redis
  namespace: one-api
type: Opaque
data:
  connection-string: cmVkaXM6Ly86cmVkaXNwYXNzd29yZEByZWRpcy1zZXJ2aWNlOjYzNzkvMA==
  # redis://:redispassword@redis-service:6379/0 (base64)
```

```bash
kubectl apply -f redis.yaml
```

> [!NOTE] > **Redis Version**: The example above uses Redis version `7-alpine`. Check the [Redis Docker Hub page](https://hub.docker.com/_/redis) for available versions and update accordingly. Consider using specific minor versions like `redis:7.4-alpine` for production environments to ensure consistency.

### Maintained Gateway API Installation

> [!WARNING]
> Do not install the retired Kubernetes ingress-nginx controller for a new deployment.
> Its maintenance ended in March 2026; selecting a later historical ingress-nginx
> tag is not a maintained solution. This warning refers to the Kubernetes
> ingress-nginx project, not every product that uses NGINX.
>
> Existing clusters need the staged migration below. Do not uninstall a shared
> controller or replace provider-owned CRDs by copying a new-install command.

This example selects **Envoy Gateway** and native Gateway API resources. The
controller and CRD charts share one version, set once in the same shell used for
all commands below:

```bash
export ENVOY_GATEWAY_VERSION=v1.9.2
```

<!-- gateway-reviewed: 2026-10-04; review-before: 2027-02-14 -->

The 2026-10-04 review selected the official v1.9.2 release. The v1.9 compatibility
matrix lists Kubernetes v1.33 through v1.36 and Gateway API v1.6.1. Choose a
Kubernetes version that is also supported by your distribution. Recheck upstream
security advisories and the compatibility matrix before installing or upgrading;
a fixed pin is not a promise of indefinite security. Re-review this selection
before 2027-02-14, the published end of support for v1.9. Do not replace it with
`latest`, a development tag, or an unreviewed data-plane image.

#### Controller and CRD ownership

Choose **exactly one** CRD path. Both require a cluster administrator to review
cluster-scoped RBAC, CRD ownership and the rendered manifests. A cloud provider's
Gateway controller is not automatically interchangeable with this controller.
The application account must not receive cluster-admin privileges.

**A. New cluster without provider-managed Gateway API CRDs:** render the pinned
standard-channel CRDs, review the output, then apply it. The command below shows
the apply step; first run the same `helm template` command without the pipe to
inspect the objects. Do not use server-side force-conflicts to take ownership.

```bash
set -euo pipefail
helm template one-api-eg-crds oci://docker.io/envoyproxy/gateway-crds-helm \
  --version "$ENVOY_GATEWAY_VERSION" \
  --set crds.gatewayAPI.enabled=true \
  --set crds.gatewayAPI.channel=standard \
  --set crds.envoyGateway.enabled=true \
  | kubectl apply --server-side -f -
```

**B. Cluster with compatible provider-managed Gateway API CRDs:** have the
platform owner verify the installed bundle version, channel, and served APIs
against the v1.9 matrix. Keep that owner; install only Envoy Gateway's extension
CRDs. Stop on incompatibility instead of layering a second Gateway API bundle.
The following read prints metadata only, not secrets:

```bash
kubectl get crd gateways.gateway.networking.k8s.io \
  -o go-template='version={{ index .metadata.annotations "gateway.networking.k8s.io/bundle-version" }} channel={{ index .metadata.annotations "gateway.networking.k8s.io/channel" }}{{ "\n" }}'
```

```bash
set -euo pipefail
helm template one-api-eg-crds oci://docker.io/envoyproxy/gateway-crds-helm \
  --version "$ENVOY_GATEWAY_VERSION" \
  --set crds.gatewayAPI.enabled=false \
  --set crds.envoyGateway.enabled=true \
  | kubectl apply --server-side -f -
```

After the selected CRD path, inspect the chart's rendered RBAC and admission
resources as part of platform review. In particular, preserve externally owned
Gateway API safe-upgrade admission policies; use the selected release's documented
chart settings when those are managed elsewhere. The following controller
installation is the default chart-ownership case, not an instruction to take over
those resources. Install the controller without reapplying either CRD bundle:

```bash
helm install one-api-eg oci://docker.io/envoyproxy/gateway-helm \
  --version "$ENVOY_GATEWAY_VERSION" \
  --namespace envoy-gateway-system \
  --create-namespace \
  --set crds.enabled=false
kubectl wait --timeout=5m --namespace envoy-gateway-system \
  deployment/envoy-gateway --for=condition=Available
```

Cloud clusters need a functioning LoadBalancer implementation and reviewed
firewall rules. Bare-metal clusters need a separately maintained load-balancer
implementation, or an explicitly designed NodePort deployment using the selected
release's EnvoyProxy settings. Do not install an old MetalLB bundle as an implicit
dependency. Qualify the chosen address allocation, source IP and firewall behavior
before publishing DNS. This example assumes the default Envoy deployment mode,
with managed proxy pods in `envoy-gateway-system`; Gateway Namespace Mode requires
a corresponding network-policy change.

#### TLS and One API routes

Have the platform's certificate manager provision `one-api-tls`, a
`kubernetes.io/tls` Secret in namespace `one-api`, with a trusted certificate for
**your** hostname. Use a maintained certificate manager and its Gateway API
integration, or your organization's existing certificate delivery process. Do not
copy private keys into this guide, Git, shell history, or diagnostic output. TLS
provisioning is required before cutover; there is no plaintext backend fallback.

Save the following as `gateway.yaml`. Replace `oneapi.yourdomain.com` consistently
in both listeners and both routes. Use a unique GatewayClass name when the
platform already owns a shared class; do not overwrite it. The HTTP listener is
for credential-free browser redirects only. API clients must use HTTPS directly:
a redirect cannot protect credentials already sent over HTTP.

```yaml
# gateway.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: one-api-envoy
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: one-api-gateway
  namespace: one-api
spec:
  gatewayClassName: one-api-envoy
  listeners:
    - name: http
      hostname: oneapi.yourdomain.com
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: Same
    - name: https
      hostname: oneapi.yourdomain.com
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - group: ''
            kind: Secret
            name: one-api-tls
      allowedRoutes:
        namespaces:
          from: Same
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: one-api-http-redirect
  namespace: one-api
spec:
  parentRefs:
    - name: one-api-gateway
      sectionName: http
  hostnames:
    - oneapi.yourdomain.com
  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            port: 443
            statusCode: 301
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: one-api-https
  namespace: one-api
spec:
  parentRefs:
    - name: one-api-gateway
      sectionName: https
  hostnames:
    - oneapi.yourdomain.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - group: ''
          kind: Service
          name: one-api-service
          port: 80
      timeouts:
        request: 300s
        backendRequest: 300s
```

The backend remains the same ClusterIP service on port 80, targeting One API on
port 3000. These bounded **total request** timeouts are not equivalent to the old
NGINX idle read timeout. Longer generations or realtime sessions need an
explicitly reviewed policy. Qualify streaming and WebSocket behavior, including
idle timeout and disconnect handling, before cutover. Do not enable retry or
request mirroring for billable POST requests; those can duplicate paid work.
Review any inherited platform retry policy as well.

No generic response buffering, credential injection, URL rewriting or external
authentication service is added by this example. Limit configuration rights for
GatewayClass, Gateway, HTTPRoute and Secrets; `allowedRoutes: Same` is not a
substitute for namespace RBAC. The example terminates public TLS at the gateway;
backend mTLS, when required by the platform, is a separate reviewed configuration.

#### Validation, migration and rollback

The following steps are **acceptance work to perform in an isolated supported
cluster**, not a claim that this guide has already passed a live cluster test.
Use a synthetic local upstream and non-production identities for billing checks.

```bash
kubectl apply --dry-run=server -f gateway.yaml
kubectl apply -f gateway.yaml
kubectl wait --timeout=5m gatewayclass/one-api-envoy --for=condition=Accepted
kubectl wait --timeout=5m --namespace one-api gateway/one-api-gateway --for=condition=Programmed
kubectl get gateway,httproute --namespace one-api
kubectl describe httproute one-api-https --namespace one-api
kubectl describe httproute one-api-http-redirect --namespace one-api
kubectl get endpointslices --namespace one-api \
  --selector=kubernetes.io/service-name=one-api-service
```

Require Gateway and listener conditions to be current, `Accepted` and
`Programmed`; both route parent statuses must be `Accepted` and `ResolvedRefs`,
with their observed generation matching the object generation. A Running pod or
successful YAML parse is not sufficient. Verify certificate chain and hostname
without insecure TLS flags, credential-free HTTP redirect, correct HTTPS backend,
all three API formats, SSE delivery beyond the default request timeout,
WebSockets, client cancellation, quota/ledger settlement and overload behavior.
Confirm an unrelated namespace cannot attach a route, backend pods cannot be
reached from an unauthorized namespace, and no control-plane or admission endpoint
is reachable from the public network. Keep admission webhooks, when the platform
uses them, reachable only from authorized API-server paths. Do not expose Envoy
admin, xDS or metrics ports as public listeners.

For migration, inventory **all** workloads using the old controller, certificate
issuers, annotations, DNS, admission objects and network policies first. Build
and qualify the new path in parallel on a separate address. Reconcile policy
semantics rather than copying NGINX annotations. Record a rollback configuration
on a maintained controller, then change traffic gradually while monitoring errors,
latency, disconnected streams and billing. Drain old streams before removal.
Do not describe reinstalling retired ingress-nginx as a secure rollback.

Uninstall the retired controller only after every dependent route has migrated
and platform ownership is established. Remove its controller-specific admission
configuration only with its owner; do not delete shared Gateway API CRDs, shared
certificates, namespaces, or policies as generic cleanup. For future Envoy
upgrades, review release notes and CRD/storage-version migrations **before** the
controller upgrade; test the candidate on a disposable supported cluster and
retain a compatible rollback path. Do not force a CRD downgrade.

#### Sources and offline regression checks

Official references checked on 2026-10-04:

- [Kubernetes ingress-nginx retirement](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/).
- [Envoy Gateway v1.9.2 release](https://github.com/envoyproxy/gateway/releases/tag/v1.9.2), [compatibility matrix](https://gateway.envoyproxy.io/news/releases/matrix/) and [Helm/CRD ownership guide](https://gateway.envoyproxy.io/docs/install/install-helm/).
- [TLS listeners](https://gateway.envoyproxy.io/docs/tasks/security/secure-gateways/), [request timeouts](https://gateway.envoyproxy.io/docs/tasks/traffic/http-timeouts/) and [EnvoyProxy deployment customization](https://gateway.envoyproxy.io/docs/tasks/operations/customize-envoyproxy/).

From the repository root, in an isolated Python environment:

```bash
python3 -m pip install -r scripts/requirements-k8s-docs.txt
python3 -m unittest discover -s scripts -p test_k8s_gateway_docs.py -v
```

These checks capture installer arguments with fake Helm/kubectl executables,
check the dated pin, and inspect parsed YAML relationships and negative controls.
They do **not** download/render the real charts, scan their images, verify live
links, or replace the cluster acceptance matrix above. A link check and real
chart/cluster qualification remain required before operational acceptance.

#### Production Considerations

##### Security

1. **Network Policies**: Restrict network traffic between pods:

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: one-api-network-policy
  namespace: one-api
spec:
  podSelector:
    matchLabels:
      app: one-api
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: envoy-gateway-system
          podSelector:
            matchLabels:
              gateway.envoyproxy.io/owning-gateway-namespace: one-api
              gateway.envoyproxy.io/owning-gateway-name: one-api-gateway
      ports:
        - protocol: TCP
          port: 3000
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: postgresql # or mysql
      ports:
        - protocol: TCP
          port: 5432 # or 3306 for MySQL
    - to:
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - protocol: TCP
          port: 6379
    - to: [] # Allow outbound internet access for AI APIs
      ports:
        - protocol: TCP
          port: 443
        - protocol: TCP
          port: 80
```

2. **Pod Security Standards**: Add security context to deployments:

```yaml
# Add to deployment.yaml under spec.template.spec
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
containers:
  - name: one-api
    # ... other config
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
```

3. **Secrets Management**: Use external secret management systems like:
   - [External Secrets Operator](https://external-secrets.io/) - Integrates with various secret backends including 1Password
   - [1Password Secrets Automation](https://developer.1password.com/docs/connect/) - Enterprise secret management with Connect API
   - [Sealed Secrets](https://sealed-secrets.netlify.app/)
   - [Vault](https://www.vaultproject.io/)

**Example: Using 1Password with External Secrets Operator**

```yaml
# 1password-secret-store.yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: onepassword-secret-store
  namespace: one-api
spec:
  provider:
    onepassword:
      connectHost: 'https://your-connect-host'
      vaults:
        Production: 1
      auth:
        secretRef:
          connectToken:
            name: onepassword-token
            key: token
---
# External secret for database credentials
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: one-api-database-external
  namespace: one-api
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: onepassword-secret-store
    kind: SecretStore
  target:
    name: one-api-database
    creationPolicy: Owner
  data:
    - secretKey: dsn
      remoteRef:
        key: 'One API Database'
        property: dsn
```

### Scaling

> [!IMPORTANT] > **Scaling Strategy for Components with Attached Storage**:
> For deployments with attached persistent storage (such as PostgreSQL, MySQL, Redis, or **One API with persistent volumes**), **vertical scaling** (increasing CPU/memory resources) is recommended rather than horizontal scaling. This is because:
>
> - PersistentVolumeClaims with `ReadWriteOnce` access mode cannot be shared across multiple pods
> - Database clustering/replication requires specific configuration and coordination
> - Horizontal scaling of stateful services can lead to data consistency issues
>
> **Horizontal scaling** (HPA) should only be used for **completely stateless components** (One API without persistent storage).

1. **Horizontal Pod Autoscaler (HPA)** (for stateless components):

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: one-api-hpa
  namespace: one-api
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: one-api
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Percent
          value: 50
          periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Percent
          value: 100
          periodSeconds: 60
```

2. **Pod Disruption Budget**:

```yaml
# pdb.yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: one-api-pdb
  namespace: one-api
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: one-api
```

### Monitoring and Logging

1. **ServiceMonitor** for Prometheus (if using Prometheus Operator):

```yaml
# servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: one-api-metrics
  namespace: one-api
  labels:
    app: one-api
spec:
  selector:
    matchLabels:
      app: one-api
  endpoints:
    - port: http
      path: /api/metrics
```

2. **Persistent Volumes** for production databases:

```yaml
# For cloud providers, use appropriate storage classes
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgresql-pvc
  namespace: one-api
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: fast-ssd # Adjust based on your cluster
  resources:
    requests:
      storage: 50Gi
```

### Deployment Commands

Deploy everything in the correct order:

```bash
# 1. Create namespace
kubectl apply -f namespace.yaml

# 2. Deploy database (PostgreSQL or MySQL)
kubectl apply -f postgresql.yaml  # or mysql.yaml

# 3. Deploy Redis (optional but recommended)
kubectl apply -f redis.yaml

# 4. Deploy One API
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml

# 5. After gateway-controller and TLS provisioning, deploy the reviewed routes
kubectl apply -f gateway.yaml

# 6. Production configurations
kubectl apply -f hpa.yaml
kubectl apply -f pdb.yaml
kubectl apply -f network-policy.yaml

# Check deployment status
kubectl get pods -n one-api
kubectl get services -n one-api
kubectl get gateway,httproute -n one-api
```

### Health Checks

Monitor your deployment:

```bash
# Check pod status
kubectl get pods -n one-api -w

# View logs
kubectl logs -f deployment/one-api -n one-api

# Check service endpoints
kubectl get endpoints -n one-api

# Test database connectivity
kubectl exec -it deployment/one-api -n one-api -- /bin/sh
# Inside container: test database connection
```

### Backup Strategy

For production environments, implement regular backups:

```bash
# PostgreSQL backup example
kubectl exec -it deployment/postgresql -n one-api -- pg_dump -U oneapi oneapi > backup-$(date +%Y%m%d).sql

# MySQL backup example
kubectl exec -it deployment/mysql -n one-api -- mysqldump -u oneapi -p oneapi > backup-$(date +%Y%m%d).sql
```

> [!NOTE] > **Production Recommendations:**
>
> - Use managed database services (RDS, Cloud SQL, etc.) for better reliability
> - Implement proper backup and disaster recovery procedures
> - Use monitoring solutions like Prometheus + Grafana
> - Consider using Helm charts for easier management
> - Implement CI/CD pipelines for automated deployments
> - Use cert-manager for automated SSL certificate management
> - Configure resource quotas and limits appropriately
> - Regularly update container images and apply security patches
