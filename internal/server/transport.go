package server

import (
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
)

type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportSSE            Transport = "sse"
	TransportStreamableHTTP Transport = "streamable-http"
)

func ParseTransport(s string) (Transport, error) {
	switch s {
	case "stdio":
		return TransportStdio, nil
	case "sse":
		return TransportSSE, nil
	case "streamable-http":
		return TransportStreamableHTTP, nil
	default:
		return "", fmt.Errorf("unknown transport %q: expected stdio, sse, or streamable-http", s)
	}
}

type TransportOpts struct {
	Transport                 Transport
	Address                   string
	BasePath                  string
	EndpointPath              string
	SessionIdleTimeoutMinutes int
	Logger                    *slog.Logger
}

func Serve(mcpServer *server.MCPServer, opts TransportOpts) error {
	switch opts.Transport {
	case TransportStdio:
		return server.ServeStdio(mcpServer)
	case TransportSSE:
		sseServer := server.NewSSEServer(mcpServer,
			server.WithBasePath(opts.BasePath),
		)
		return sseServer.Start(opts.Address)
	case TransportStreamableHTTP:
		httpServer := server.NewStreamableHTTPServer(mcpServer,
			server.WithEndpointPath(opts.EndpointPath),
		)
		return httpServer.Start(opts.Address)
	default:
		return fmt.Errorf("unsupported transport %q", opts.Transport)
	}
}
