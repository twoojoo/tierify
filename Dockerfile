# Multi-stage build for production-ready Tierify deployment
# Stage 1: Build the application
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /build

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
# - CGO_ENABLED=1: Required for SQLite
# - -ldflags: Strip debug info and reduce binary size
# - -trimpath: Remove file system paths from the binary
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static'" \
    -trimpath \
    -o tierify \
    ./cmd/tierify

# Stage 2: Create minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite-libs \
    && addgroup -g 1000 tierify \
    && adduser -D -u 1000 -G tierify tierify

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/tierify /app/tierify

# Copy migrations (if needed for future migration tooling)
COPY --from=builder /build/internal/db/sqlite/migrations /app/migrations

# Create data directory for SQLite database
RUN mkdir -p /app/data && chown -R tierify:tierify /app

# Switch to non-root user
USER tierify

# Environment variables with defaults
ENV PORT=8080 \
    HOST=0.0.0.0 \
    DB_TYPE=sqlite \
    DB_CONNECTION_STRING=/app/data/tierify.db \
    LOG_LEVEL=info \
    LOG_FORMAT=json \
    ALLOW_USAGE_BELOW_ZERO=false

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["/app/tierify"]
