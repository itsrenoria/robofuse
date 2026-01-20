FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o robofuse ./cmd/robofuse

# Final stage
FROM python:3.12-alpine

# Install dependencies
RUN apk --no-cache add ca-certificates tzdata git

WORKDIR /app

# Copy Go binary from builder
COPY --from=builder /app/robofuse .

# Copy Python scripts and requirements
COPY scripts/ ./scripts/
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Create directories
RUN mkdir -p /data /app/Library /app/Organized /app/cache

# Volume for config
VOLUME ["/data"]

# Volume for STRM output
VOLUME ["/app/Library"]

# Volume for organized output
VOLUME ["/app/Organized"]

# Volume for cache
VOLUME ["/app/cache"]

ENTRYPOINT ["./robofuse"]
CMD ["watch", "--config", "/data/config.json"]
