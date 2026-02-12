FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(cat VERSION 2>/dev/null || echo dev)" \
    -o /redis-pubsub-exporter \
    ./cmd/redis-pubsub-exporter

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /redis-pubsub-exporter /redis-pubsub-exporter

EXPOSE 9123

USER 65534

ENTRYPOINT ["/redis-pubsub-exporter"]
