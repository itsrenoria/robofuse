FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o robofuse ./cmd/robofuse

# Final stage - pure Alpine (no Python needed!)
FROM alpine:3.19

# Install minimal dependencies
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy Go binary from builder
COPY --from=builder /app/robofuse .

# Create directories
RUN mkdir -p /data /app/library /app/library-organized /app/cache

# Volume for config
VOLUME ["/data"]

# Volume for STRM output
VOLUME ["/app/library"]

# Volume for organized output
VOLUME ["/app/library-organized"]

# Volume for cache
VOLUME ["/app/cache"]

ENTRYPOINT ["./robofuse"]
CMD ["watch", "--config", "/data/config.json"]
