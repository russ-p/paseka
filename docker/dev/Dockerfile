# syntax=docker/dockerfile:1
# Minimal apiary-like image: Ubuntu 24.04, Go, git, Cursor Agent CLI, prebuilt paseka.
# Build from repo root:
#   docker build -f docker/dev/Dockerfile -t paseka-dev:local .
#
# The Queen Console Next bundle is a build artifact, not a committed file, so the
# web-builder stage below produces it inside the image before Go compiles. Nothing
# from the host's internal/console/next/dist reaches the Go stage (.dockerignore).

ARG GO_VERSION=1.25.0
ARG UBUNTU_VERSION=24.04

FROM node:24-bookworm-slim AS web-builder

# pnpm's version comes from web/package.json `packageManager`, so there is one
# place to bump it.
RUN corepack enable

WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY web/ ./
RUN pnpm build

# ── build paseka once into the image ──────────────────────────────────────────
FROM ubuntu:${UBUNTU_VERSION} AS builder

ARG GO_VERSION
ARG TARGETARCH

# The build stamp. `.dockerignore` keeps `.git` out of the context, so the
# toolchain has no checkout to read a commit from and `paseka version` inside the
# image would answer `dev` with nothing behind it. These carry it instead:
#
#   docker build -f docker/dev/Dockerfile \
#     --build-arg VERSION=0.6.0 --build-arg COMMIT="$(git rev-parse HEAD)" \
#     --build-arg COMMIT_DATE="$(git show -s --format=%cI HEAD)" .
#
# Left empty they are omitted from the link, and the binary falls back to whatever
# build info the toolchain embedded — which is `dev`, honestly reported.
ARG VERSION=""
ARG COMMIT=""
ARG COMMIT_DATE=""
ARG TREE_STATE=""

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git \
    && rm -rf /var/lib/apt/lists/*

RUN ARCH=$(case "${TARGETARCH}" in amd64) echo amd64;; arm64) echo arm64;; *) echo amd64;; esac) \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" \
    | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}" \
    GOPATH=/go \
    CGO_ENABLED=0

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The only source of the embed tree: the bundle built above. The checked-in
# fallback is all a build without it would have, so a missing one is a build error
# rather than a silently legacy-only binary.
COPY --from=web-builder /src/internal/console/next/dist ./internal/console/next/dist
# An empty stamp is left off the link rather than written as an empty string, so
# the binary reads as unstamped instead of as a build with a blank version.
RUN set -eu; \
    ldflags="-s -w"; \
    for pair in "version=${VERSION}" "commit=${COMMIT}" "date=${COMMIT_DATE}" "dirty=${TREE_STATE}"; do \
      name="${pair%%=*}"; value="${pair#*=}"; \
      if [ -n "${value}" ]; then \
        ldflags="${ldflags} -X github.com/russ-p/paseka/internal/version.${name}=${value}"; \
      fi; \
    done; \
    go build -ldflags "${ldflags}" -o /out/paseka ./cmd/paseka

# ── runtime = host-like minimal toolbelt ──────────────────────────────────────
FROM ubuntu:${UBUNTU_VERSION}

ARG GO_VERSION
ARG TARGETARCH
ARG DEV_UID=1000
ARG DEV_GID=1000

ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/home/dev \
    PATH="/home/dev/.local/bin:/usr/local/go/bin:/usr/local/bin:${PATH}" \
    GOPATH=/home/dev/go \
    XDG_CONFIG_HOME=/home/dev/.config

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git bash \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g "${DEV_GID}" dev \
    && useradd -m -u "${DEV_UID}" -g "${DEV_GID}" -s /bin/bash dev

# Go toolchain (rebuild inside the container if needed)
RUN ARCH=$(case "${TARGETARCH}" in amd64) echo amd64;; arm64) echo arm64;; *) echo amd64;; esac) \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" \
    | tar -C /usr/local -xz

# Prebuilt binary lives outside the repo volume mount
COPY --from=builder /out/paseka /usr/local/bin/paseka

USER dev
WORKDIR /home/dev

# Cursor Agent CLI → ~/.local/bin/agent
RUN curl -fsS https://cursor.com/install | bash \
 && agent --version

RUN mkdir -p \
      /home/dev/workspace \
      /home/dev/.config/paseka \
      /home/dev/.cursor \
      /home/dev/.config/cursor

WORKDIR /home/dev/workspace

# Secrets via runtime env only (CURSOR_API_KEY, PASEKA_NATS_URL, …)
EXPOSE 8787

# Bind all interfaces so published host ports reach Queen Console
CMD ["paseka", "console", "--addr", "0.0.0.0:8787"]
