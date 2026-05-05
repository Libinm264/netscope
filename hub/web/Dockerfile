# syntax=docker/dockerfile:1.7
#
# Multi-arch Next.js image (linux/amd64 + linux/arm64)
#
# Key design decision: deps and builder stages are pinned to $BUILDPLATFORM
# (always the CI host — amd64).  Next.js produces pure-JS output, so those
# artifacts are safe to copy into the target-platform (arm64) runner image.
# This avoids running `npm install` or `next build` under QEMU emulation,
# which causes illegal-instruction crashes on Node 20 and is 10-20× slower.

# ── Stage 1: install dependencies ────────────────────────────────────────────
# Runs on the BUILD host — never under QEMU.
FROM --platform=$BUILDPLATFORM node:20-alpine AS deps
WORKDIR /app

# Copy both manifests so npm ci can verify the lockfile.
COPY package.json package-lock.json ./

# --mount=type=cache keeps the npm package store across rebuilds so that only
# changed packages are re-downloaded when the lockfile changes.
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# ── Stage 2: build ────────────────────────────────────────────────────────────
# Also on the build host — output is platform-agnostic JS.
FROM --platform=$BUILDPLATFORM node:20-alpine AS builder
WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1
# next.config.mjs sets output:"standalone" — produces .next/standalone
RUN npm run build

# ── Stage 3: runtime ──────────────────────────────────────────────────────────
# This stage IS the target platform (amd64 or arm64).
# We only copy pre-built JS files — no build tools, no QEMU issues.
FROM node:20-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs && \
    adduser  --system --uid 1001 nextjs

COPY --from=builder /app/public                                  ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone  ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static      ./.next/static

USER nextjs

EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
