# syntax=docker/dockerfile:1.7
#
# Multi-arch Go image (linux/amd64 + linux/arm64)
#
# The builder stage is pinned to $BUILDPLATFORM so the Go toolchain always
# runs natively on the CI host (amd64).  CGO_ENABLED=0 + GOOS/GOARCH lets Go
# cross-compile for the target arch without QEMU — producing a correct, fully
# static binary that is then copied into the minimal target-platform runtime.

# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

# Declare the target platform args injected by docker buildx.
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Layer 1: download module graph (cached until go.mod / go.sum change).
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

# Layer 2: compile — cross-compile natively with CGO disabled.
COPY . .
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -trimpath -o /hub-api .

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
# Minimal target-platform image — just the static binary.
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app
COPY --from=builder /hub-api /app/hub-api

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget --spider -q http://localhost:8080/health || exit 1

CMD ["/app/hub-api"]
