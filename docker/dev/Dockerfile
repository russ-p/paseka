# syntax=docker/dockerfile:1
# Minimal apiary-like image: Ubuntu 24.04, Go, git, Cursor Agent CLI, prebuilt paseka.
# Build from repo root:
#   docker build -f docker/dev/Dockerfile -t paseka-dev:local .

ARG GO_VERSION=1.25.0
ARG UBUNTU_VERSION=24.04

# ── build paseka once into the image ──────────────────────────────────────────
FROM ubuntu:${UBUNTU_VERSION} AS builder

ARG GO_VERSION
ARG TARGETARCH

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
RUN go build -o /out/paseka ./cmd/paseka

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
