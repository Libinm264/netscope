# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Layer 1: download deps declared in go.sum (cached unless go.mod/go.sum change).
COPY go.mod go.sum ./
RUN go mod download

# Layer 2: copy source, tidy (resolves any packages added to go.mod after the
# last committed go mod tidy), then compile.
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
