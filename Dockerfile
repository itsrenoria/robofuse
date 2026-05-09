# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o robofuse ./cmd/robofuse

# Final stage - minimal runtime image
FROM alpine:3.21@sha256:a8560b36e8b8210634f77d9f7f9efd7ffa463e380b75e2e74aff4511df3ef88c

# Install minimal dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN adduser -D -h /app robofuse

WORKDIR /app

# Copy Go binary from builder
COPY --from=builder /app/robofuse .

# Create directories and set ownership
RUN mkdir -p /data /app/library /app/library-organized /app/cache && \
    chown -R robofuse:robofuse /app /data

# Switch to non-root user
USER robofuse

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
