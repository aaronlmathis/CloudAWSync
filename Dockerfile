# Multi-stage build for CloudAWSync
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o cloudawsync .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -S cloudawsync && adduser -S cloudawsync -G cloudawsync

# Create directories
RUN mkdir -p /etc/cloudawsync /var/log/cloudawsync /data

# Copy binary from builder
COPY --from=builder /app/cloudawsync /usr/local/bin/cloudawsync

# Copy example configuration
COPY config.yaml.example /etc/cloudawsync/config.yaml.example

# Set ownership
RUN chown -R cloudawsync:cloudawsync /etc/cloudawsync /var/log/cloudawsync /data

# Switch to non-root user
USER cloudawsync

# Expose metrics port
EXPOSE 9090

# Set default command
CMD ["cloudawsync", "-config", "/etc/cloudawsync/config.yaml"]

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9090/metrics || exit 1

# Labels
LABEL maintainer="Aaron Mathis <aaron@deepthought.sh>"
LABEL description="CloudAWSync - Cloud File Synchronization Agent"
LABEL version="1.0.0"
LABEL license="GPL-3.0-or-later"
