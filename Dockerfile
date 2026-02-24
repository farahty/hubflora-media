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
FROM alpine:3.21

# Install libvips runtime
RUN apk add --no-cache vips ca-certificates

COPY --from=builder /hubflora-media /hubflora-media

EXPOSE 8090

ENTRYPOINT ["/hubflora-media"]
