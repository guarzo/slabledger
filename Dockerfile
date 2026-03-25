# Production Dockerfile for SlabLedger
# Multi-stage build for minimal image size

# ==============================================================================
# Frontend Build Stage
# ==============================================================================
FROM node:24-alpine AS frontend-builder

WORKDIR /app/web

# Copy package files first (better caching)
COPY web/package.json web/package-lock.json ./

# Install dependencies
RUN npm ci

# Copy frontend source
COPY web/ ./

# Build frontend
RUN npm run build

# ==============================================================================
# Go Build Stage
# ==============================================================================
FROM golang:1.26-alpine3.23 AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev \
    sqlite-dev

WORKDIR /app

# Copy dependency files first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build application
# CGO_ENABLED=1 required for SQLite
# Using -extldflags "-static" for fully static binary
ARG VERSION=dev
ENV CGO_CFLAGS="-D_LARGEFILE64_SOURCE"
RUN CGO_ENABLED=1 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-w -s -extldflags '-static' -X github.com/guarzo/slabledger/internal/platform/config.Version=${VERSION}" \
    -tags 'sqlite_omit_load_extension netgo osusergo' \
    -o slabledger \
    ./cmd/slabledger

# ==============================================================================
# Runtime Stage
# ==============================================================================
FROM alpine:3.23

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=app:app /app/slabledger .

# Copy built frontend assets from frontend-builder
COPY --from=frontend-builder --chown=app:app /app/web/dist ./web/dist

# Create necessary directories with correct ownership
# These will be used as mount points for volumes
RUN mkdir -p /app/data /app/cache && \
    chown -R app:app /app

# Switch to non-root user
USER app

# Expose port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/health || exit 1

# Run application
ENTRYPOINT ["./slabledger"]
CMD ["--web", "--port", "8081"]
