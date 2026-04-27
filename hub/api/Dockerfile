# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Download dependencies.
# -mod=mod allows go mod download to fetch packages added to go.mod after the
# last `go mod tidy` run and to add their hashes to go.sum within the layer.
# This keeps the build hermetic even when go.sum is temporarily behind go.mod.
COPY go.mod go.sum ./
RUN go mod download -mod=mod

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -ldflags="-s -w" -trimpath -o /hub-api .

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app
COPY --from=builder /hub-api /app/hub-api

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget --spider -q http://localhost:8080/health || exit 1

CMD ["/app/hub-api"]
