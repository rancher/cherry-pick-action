# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /workspace

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -o /cherry-pick-action \
    ./cmd/cherry-pick-action

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    git \
    gnupg \
    ca-certificates

# Copy binary from builder
COPY --from=builder /cherry-pick-action /usr/local/bin/cherry-pick-action

# Create a non-root user
RUN addgroup -S action && adduser -S action -G action
USER action

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/cherry-pick-action"]
