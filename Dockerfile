## syntax=docker/dockerfile:1.7
# Multi-stage build for one-api with web UI and ffmpeg support
# Usage examples:
#   docker buildx build --platform linux/amd64,linux/arm64 -t yourrepo/one-api:latest .

ARG NODE_IMAGE=node:24-bookworm@sha256:6449e19d8184d84062525b6a7524def03146c2cb04a7f08b076680d0e8e4240a
ARG GO_IMAGE=golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5
ARG FFMPEG_IMAGE=linuxserver/ffmpeg:latest@sha256:742e17516dd4d7b76b939d774bd08f0f6b7cab8bce1c014b95ab2fb992fd0a1f
ARG RUNTIME_IMAGE=ubuntu:26.04@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944

#############################
# Stage 1: Frontend build   #
#############################
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web-builder
WORKDIR /web

# Public Sentry settings are loaded from the runtime API, never embedded as build arguments.
# Copy sources (place themes directly under /web)
COPY web/ ./

# Install & build each theme sequentially to avoid OOM in CI
ENV YARN_ENABLE_IMMUTABLE_INSTALLS=0
RUN set -e; for theme in berry air modern; do \
        echo "==> installing deps for $theme"; \
        (cd /web/$theme && yarn install --network-timeout 600000); \
    done

RUN mkdir -p /web/build
ENV DISABLE_ESLINT_PLUGIN=true
RUN set -e; BUILD_ID=$(date +%s); \
        for theme in berry air modern; do \
                echo "==> building $theme (build_id=$BUILD_ID)"; \
                REACT_APP_VERSION=$BUILD_ID yarn --cwd /web/$theme run build; \
        done

############################
# Stage 2: Go build        #
############################
FROM ${GO_IMAGE} AS go-builder
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

RUN --mount=type=cache,target=/root/.cache/go-build \
        echo "Building one-api for ${TARGETOS:-linux}/${TARGETARCH:-$(go env GOARCH)}" && \
        GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} \
        go build -trimpath -ldflags "-s -w" -o /out/one-api

#############################
# Stage 3: Media runtime     #
#############################
FROM ${FFMPEG_IMAGE} AS media-runtime
RUN apt-get update && apt-get install -y --no-install-recommends python3-minimal \
    && rm -rf /var/lib/apt/lists/*
COPY scripts/collect_media_runtime.py /collect_media_runtime.py
RUN python3 /collect_media_runtime.py

#############################
# Stage 4: Runtime image     #
#############################
FROM ${RUNTIME_IMAGE} AS final
COPY --from=media-runtime /media-root/ /
ARG TARGETARCH
LABEL org.opencontainers.image.title="one-api" \
            org.opencontainers.image.source="https://github.com/Laisky/one-api" \
            org.opencontainers.image.licenses="MIT"

ENV DEBIAN_FRONTEND=noninteractive \
        TZ=Etc/UTC

RUN set -e; \
        printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' > /etc/apt/apt.conf.d/80-retries; \
        apt-get update; \
        apt-get install -y --no-install-recommends \
                bash ca-certificates tzdata curl libsqlite3-0 gosu; \
        cmp /etc/nsswitch.conf /opt/oneapi-media/nsswitch.conf; \
        ldconfig; \
        ffmpeg -hide_banner -version >/dev/null; \
        ffprobe -hide_banner -version >/dev/null; \
        rm -rf /var/lib/apt/lists/*

ARG ONEAPI_UID=10001
ARG ONEAPI_GID=10001

EXPOSE 3000

# Create dedicated user with deterministic UID/GID so host can pre‑chown bind mount.
RUN groupadd --system --gid ${ONEAPI_GID} oneapi && \
        useradd  --system --no-create-home --home /data --uid ${ONEAPI_UID} --gid ${ONEAPI_GID} \
                        --shell /usr/sbin/nologin oneapi && \
        mkdir -p /data && chown oneapi:oneapi /data

# Set ownership while copying so a later chown does not duplicate the binary layer.
COPY --from=go-builder --chown=${ONEAPI_UID}:${ONEAPI_GID} /out/one-api /one-api

# Add entrypoint script with its runtime mode in the same layer.
COPY --chmod=755 scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

WORKDIR /data

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://127.0.0.1:3000/api/status || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
