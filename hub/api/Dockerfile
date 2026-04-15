# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Copy source first, then tidy — handles missing go.sum on fresh builds
COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /hub-api .

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app
COPY --from=builder /hub-api /app/hub-api

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget --spider -q http://localhost:8080/health || exit 1

CMD ["/app/hub-api"]
