# Build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=0
RUN go build -ldflags='-w -s' -o /build/grafana-oncall-mcp ./cmd/oncall-mcp

# Final stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/grafana-oncall-mcp /usr/local/bin/grafana-oncall-mcp

ENTRYPOINT ["/usr/local/bin/grafana-oncall-mcp"]
