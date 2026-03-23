# Stage 1: Builder
FROM golang:1.23-alpine AS builder

# Install system dependencies (needed for some CGO parts if any, though we aim for static)
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy dependency files first for caching optimality
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire source code
COPY . .

# Build the binary for the Ingestor (Main entry point)
# -ldflags="-s -w" to strip debug symbols and reduce size
# CGO_ENABLED=0 for a purely static binary (distroless/alpine compatible)
RUN CGO_ENABLED=0 GOOS=linux go build -o ingestor -ldflags="-s -w" ./cmd/ingestor/main.go

# Stage 2: Final Runtime Image
FROM alpine:latest

# Install CA certificates for secure PubSub/Sentry communication
RUN apk add --no-cache ca-certificates

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/ingestor .

# Expose the default port (Cloud Run sets $PORT at runtime)
EXPOSE 8080

# Execution Command
ENTRYPOINT ["./ingestor"]
