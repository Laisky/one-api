## syntax=docker/dockerfile:1.7
# Multi-stage build for one-api with web UI and ffmpeg support
# Usage examples:
#   docker buildx build --platform linux/amd64,linux/arm64 -t yourrepo/one-api:latest .

ARG NODE_IMAGE=node:24-bookworm
ARG GO_IMAGE=golang:1.25.3-bookworm
ARG FFMPEG_IMAGE=linuxserver/ffmpeg:latest

#############################
# Stage 1: Frontend build   #
#############################
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web-builder
WORKDIR /web

# Copy version & sources (place themes directly under /web)
COPY VERSION ./
COPY web/ ./

# Install & build each theme sequentially to avoid OOM in CI
ENV YARN_ENABLE_IMMUTABLE_INSTALLS=0
RUN set -e; for theme in default berry air modern; do \
        echo "==> installing deps for $theme"; \
        (cd /web/$theme && yarn install --network-timeout 600000); \
    done

RUN mkdir -p /web/build
ENV DISABLE_ESLINT_PLUGIN=true
RUN set -e; export REACT_APP_VERSION=$(cat VERSION); \
        for theme in default berry air modern; do \
                echo "==> building $theme (version=$REACT_APP_VERSION)"; \
                npm run build --prefix /web/$theme; \
        done

############################
# Stage 2: Go build        #
############################
FROM --platform=$TARGETPLATFORM ${GO_IMAGE} AS go-builder
ARG TARGETOS
ARG TARGETARCH
ENV TZ=Etc/UTC \
        CGO_ENABLED=1 \
        GO111MODULE=on

RUN set -e; \
        printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' > /etc/apt/apt.conf.d/80-retries; \
        # Add an additional mirror file (keep base list intact for fallback)
        echo 'deb http://deb.debian.org/debian bookworm main' > /etc/apt/sources.list.d/99-extra.list; \
        apt-get update; \
        apt-get install -y --no-install-recommends sqlite3 libsqlite3-dev ca-certificates; \
        rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
COPY --from=web-builder /web/build ./web/build

# Build (Version embedded). Reading from VERSION file copied above.
RUN --mount=type=cache,target=/root/.cache/go-build \
        VERSION=$(cat VERSION) && \
        echo "Building one-api for ${TARGETOS:-linux}/${TARGETARCH:-$(go env GOARCH)} version=$VERSION" && \
        GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} \
        go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/Laisky/one-api/common.Version=$VERSION" -o /out/one-api

#############################
# Stage 3: Runtime image    #
#############################
FROM ubuntu:24.04 AS runtime
LABEL org.opencontainers.image.title="one-api" \
            org.opencontainers.image.source="https://github.com/Laisky/one-api" \
            org.opencontainers.image.licenses="MIT"

ENV DEBIAN_FRONTEND=noninteractive \
        TZ=Etc/UTC

RUN set -e; \
        printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' > /etc/apt/apt.conf.d/80-retries; \
        if [ "$TARGETARCH" = "amd64" ]; then \
            echo 'deb http://archive.ubuntu.com/ubuntu noble main restricted universe multiverse' > /etc/apt/sources.list.d/99-extra.list; \
            echo 'deb https://mirrors.kernel.org/ubuntu noble main restricted universe multiverse' >> /etc/apt/sources.list.d/99-extra.list; \
        fi; \
        apt-get update; \
        apt-get install -y --no-install-recommends \
                ca-certificates tzdata curl libsqlite3-0; \
        rm -rf /var/lib/apt/lists/*

# Pull in ffmpeg & ffprobe
FROM --platform=$TARGETPLATFORM ${FFMPEG_IMAGE} AS ffmpeg

FROM runtime AS final
COPY --from=ffmpeg /usr/local/bin/ffmpeg /usr/local/bin/ffprobe /usr/local/bin/
COPY --from=go-builder /out/one-api /one-api

EXPOSE 3000

ARG ONEAPI_UID=10001
ARG ONEAPI_GID=10001
# Create dedicated user with deterministic UID/GID so host can pre‑chown bind mount.
RUN groupadd --system --gid ${ONEAPI_GID} oneapi && \
        useradd  --system --no-create-home --home /data --uid ${ONEAPI_UID} --gid ${ONEAPI_GID} \
                        --shell /usr/sbin/nologin oneapi && \
        mkdir -p /data && chown oneapi:oneapi /one-api

# Install gosu for privilege drop (tiny init not strictly needed; keeping minimal change)
RUN set -e; apt-get update; apt-get install -y --no-install-recommends gosu; rm -rf /var/lib/apt/lists/*

# Add entrypoint script
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

WORKDIR /data

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://127.0.0.1:3000/api/status || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
