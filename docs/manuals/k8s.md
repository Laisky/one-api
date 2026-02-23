# Kubernetes Deployment

<p align="center">
   <img src="https://kubernetes.io/images/kubernetes.png" alt="sailing-with-k8s" width="80">
</p>

This section provides comprehensive instructions for deploying One API on Kubernetes with various configurations.

## Prerequisites

- Kubernetes cluster (v1.20+ or later)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) configured to communicate with your cluster
- [`helm`](https://helm.sh/docs/intro/install/) (optional, for package management)

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

### NGINX Ingress Controller Installation

Before configuring Ingress for One API, you need to install an Ingress Controller. This section covers installing NGINX Ingress Controller, which is one of the most popular choices.

#### For Cloud Providers

###### Google Kubernetes Engine (GKE)

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/cloud/deploy.yaml
```

###### Amazon EKS

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/aws/deploy.yaml
```

###### Azure Kubernetes Service (AKS)

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/cloud/deploy.yaml
```

###### DigitalOcean Kubernetes

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/do/deploy.yaml
```

###### Oracle Cloud Infrastructure (OCI)

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/cloud/deploy.yaml
```

###### Scaleway

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/scw/deploy.yaml
```

###### Exoscale

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/exoscale/deploy.yaml
```

###### OVHcloud

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/ovhcloud/deploy.yaml
```

> [!NOTE] > **NGINX Ingress Controller Version**: The examples above use version `v1.8.4`. Always check the [NGINX Ingress Controller releases page](https://github.com/kubernetes/ingress-nginx/releases) for the latest stable version and update the URLs accordingly. Replace `controller-v1.8.4` with the latest version tag (e.g., `controller-v1.11.2` or newer).

#### For Bare Metal / On-Premises

###### Using NodePort

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/baremetal/deploy.yaml
```

###### Using MetalLB (Recommended for Bare Metal)

First, install MetalLB for LoadBalancer support:

```yaml
# metallb-namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: metallb-system
  labels:
    name: metallb-system
```

```bash
kubectl apply -f metallb-namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.12/config/manifests/metallb-native.yaml
```

> [!NOTE] > **MetalLB Version**: The example above uses MetalLB version `v0.13.12`. Check the [MetalLB releases page](https://github.com/metallb/metallb/releases) for the latest stable version and update the URL accordingly. Replace `v0.13.12` with the latest version tag (e.g., `v0.14.8` or newer).

Configure MetalLB IP address pool:

```yaml
# metallb-config.yaml
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: first-pool
  namespace: metallb-system
spec:
  addresses:
    - 192.168.1.240-192.168.1.250 # Adjust to your network
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: example
  namespace: metallb-system
spec:
  ipAddressPools:
    - first-pool
```

```bash
kubectl apply -f metallb-config.yaml
```

Then install NGINX Ingress Controller:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.4/deploy/static/provider/cloud/deploy.yaml
```

#### Using Helm (Alternative Installation Method)

Add the NGINX Ingress Controller Helm repository:

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update
```

Install NGINX Ingress Controller with Helm:

```bash
# For cloud providers with LoadBalancer support
helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace

# For bare metal with NodePort
helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace \
  --set controller.service.type=NodePort

# For bare metal with MetalLB
helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace \
  --set controller.service.type=LoadBalancer
```

#### Custom Configuration

For production environments, you may want to customize the NGINX Ingress Controller:

```yaml
# nginx-ingress-custom.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-configuration
  namespace: ingress-nginx
  labels:
    app.kubernetes.io/name: ingress-nginx
    app.kubernetes.io/part-of: ingress-nginx
data:
  # Increase proxy buffer sizes for large requests
  proxy-buffer-size: '16k'
  proxy-buffers-number: '8'
  # Enable compression
  use-gzip: 'true'
  gzip-level: '6'
  gzip-types: 'text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript'

  # Security headers
  add-base-url: 'true'
  enable-real-ip: 'true'

  # Connection settings
  keep-alive-requests: '10000'
  upstream-keepalive-connections: '50'
  upstream-keepalive-requests: '100'

  # Rate limiting (optional)
  rate-limit-rpm: '300'
  rate-limit-connections: '10'

  # Client settings
  client-max-body-size: '100m'
  client-body-buffer-size: '1m'

  # SSL settings
  ssl-protocols: 'TLSv1.2 TLSv1.3'
  ssl-ciphers: 'ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305'
```

```bash
kubectl apply -f nginx-ingress-custom.yaml
```

#### Verify Installation

Check that the NGINX Ingress Controller is running:

```bash
# Check pods
kubectl get pods -n ingress-nginx

# Check services
kubectl get svc -n ingress-nginx

# Check ingress class
kubectl get ingressclass

# For LoadBalancer service, get external IP
kubectl get svc ingress-nginx-controller -n ingress-nginx
```

Expected output should show the controller pod running and service with an external IP (for cloud providers):

```
NAME                                      READY   STATUS    RESTARTS   AGE
ingress-nginx-controller-xxx-xxx          1/1     Running   0          5m
ingress-nginx-admission-create-xxx        0/1     Completed 0          5m
ingress-nginx-admission-patch-xxx         0/1     Completed 1          5m
```

#### Test the Installation

Create a simple test to verify the ingress controller is working:

```yaml
# test-app.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
        - name: test-app
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: test-app-service
  namespace: default
spec:
  selector:
    app: test-app
  ports:
    - port: 80
      targetPort: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-app-ingress
  namespace: default
  annotations:
    kubernetes.io/ingress.class: nginx
spec:
  rules:
    - host: test.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: test-app-service
                port:
                  number: 80
```

```bash
# Deploy test app
kubectl apply -f test-app.yaml

# Test (replace with your actual ingress IP)
curl -H "Host: test.local" http://YOUR-INGRESS-IP

# Clean up test
kubectl delete -f test-app.yaml
```

#### SSL Certificate Management (Optional)

Install cert-manager for automatic SSL certificate management:

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.3/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=ready pod -l app=cert-manager -n cert-manager --timeout=60s
kubectl wait --for=condition=ready pod -l app=cainjector -n cert-manager --timeout=60s
kubectl wait --for=condition=ready pod -l app=webhook -n cert-manager --timeout=60s
```

> [!NOTE] > **cert-manager Version**: The example above uses cert-manager version `v1.13.3`. Check the [cert-manager releases page](https://github.com/cert-manager/cert-manager/releases) for the latest stable version and update the URL accordingly. Replace `v1.13.3` with the latest version tag (e.g., `v1.16.1` or newer).

Create a ClusterIssuer for Let's Encrypt:

```yaml
# letsencrypt-issuer.yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com # Replace with your email
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            class: nginx
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: your-email@example.com # Replace with your email
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
      - http01:
          ingress:
            class: nginx
```

```bash
kubectl apply -f letsencrypt-issuer.yaml
```

#### Troubleshooting

Common issues and solutions:

1. **Ingress Controller not starting**:

   ```bash
   # Check logs
   kubectl logs -n ingress-nginx deployment/ingress-nginx-controller

   # Check events
   kubectl get events -n ingress-nginx --sort-by=.metadata.creationTimestamp
   ```

2. **External IP pending (for LoadBalancer)**:
   - On cloud providers: Check if LoadBalancer service is supported
   - On bare metal: Install MetalLB or use NodePort service type

3. **Ingress not working**:

   ```bash
   # Check ingress resource
   kubectl describe ingress <ingress-name> -n <namespace>

   # Check service endpoints
   kubectl get endpoints -n <namespace>

   # Debug from inside cluster
   kubectl exec -it <any-pod> -- curl http://<service-name>.<namespace>:80
   ```

4. **SSL certificate issues**:

   ```bash
   # Check certificate status
   kubectl get certificates -A
   kubectl describe certificate <cert-name> -n <namespace>
   # Check cert-manager logs
   kubectl logs -n cert-manager deployment/cert-manager
   ```

5. **Rate limiting or connection issues**:
   - Adjust the NGINX configuration ConfigMap as shown above
   - Monitor NGINX metrics and logs for insights

Now your cluster is ready for the One API Ingress configuration!

#### Ingress Configuration

To expose One API to the internet, configure an Ingress:

##### NGINX Ingress

```yaml
# ingress-nginx.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: one-api-ingress
  namespace: one-api
  annotations:
    kubernetes.io/ingress.class: nginx
    nginx.ingress.kubernetes.io/ssl-redirect: 'true'
    nginx.ingress.kubernetes.io/force-ssl-redirect: 'true'
    nginx.ingress.kubernetes.io/proxy-body-size: '100m'
    nginx.ingress.kubernetes.io/proxy-read-timeout: '300'
    nginx.ingress.kubernetes.io/proxy-send-timeout: '300'
    cert-manager.io/cluster-issuer: 'letsencrypt-prod' # If using cert-manager
spec:
  tls:
    - hosts:
        - oneapi.yourdomain.com
      secretName: one-api-tls
  rules:
    - host: oneapi.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: one-api-service
                port:
                  number: 80
```

##### Traefik Ingress

```yaml
# ingress-traefik.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: one-api-ingress
  namespace: one-api
  annotations:
    kubernetes.io/ingress.class: traefik
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: 'true'
    traefik.ingress.kubernetes.io/router.middlewares: default-redirect-https@kubernetescrd
spec:
  tls:
    - hosts:
        - oneapi.yourdomain.com
      secretName: one-api-tls
  rules:
    - host: oneapi.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: one-api-service
                port:
                  number: 80
```

```bash
kubectl apply -f ingress-nginx.yaml  # or ingress-traefik.yaml
```

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
              name: ingress-nginx # Adjust based on your ingress controller
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

# 5. Deploy Ingress
kubectl apply -f ingress-nginx.yaml  # or ingress-traefik.yaml

# 6. Production configurations
kubectl apply -f hpa.yaml
kubectl apply -f pdb.yaml
kubectl apply -f network-policy.yaml

# Check deployment status
kubectl get pods -n one-api
kubectl get services -n one-api
kubectl get ingress -n one-api
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
