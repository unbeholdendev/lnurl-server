# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o lnurl-server ./cmd/lnurl-server

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=builder /build/lnurl-server /app/lnurl-server

USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/app/lnurl-server", "-healthcheck"]

ENTRYPOINT ["/app/lnurl-server"]
CMD ["-config", "/app/config.yaml"]
