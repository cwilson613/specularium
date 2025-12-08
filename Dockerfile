# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies for CGO (SQLite)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files and download dependencies
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Tidy modules and build the application with CGO enabled for SQLite
RUN go mod tidy && CGO_ENABLED=1 GOOS=linux go build -o specularium -ldflags="-s -w" ./cmd/server

# Production stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/specularium /usr/local/bin/

# Create data directory
RUN mkdir -p /data

# Expose port
EXPOSE 3000

# Set environment variables
ENV TZ=UTC

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/ || exit 1

# Run as non-root user
RUN adduser -D -u 1000 specularium
RUN chown -R specularium:specularium /data
USER specularium

# Start the application
ENTRYPOINT ["specularium"]
CMD ["-addr", ":3000", "-db", "/data/specularium.db"]
