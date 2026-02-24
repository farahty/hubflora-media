# Build stage
FROM golang:1.24-alpine AS builder

# Install libvips for bimg
RUN apk add --no-cache vips-dev gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=1 go build -o /hubflora-media ./cmd/server

# Runtime stage
FROM debian:bookworm-slim

# Install libvips runtime
RUN apt-get update && apt-get install -y \
    libvips42 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /hubflora-media /hubflora-media

EXPOSE 8090

ENTRYPOINT ["/hubflora-media"]
