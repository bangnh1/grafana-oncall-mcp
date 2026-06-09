package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	mcpServer *server.MCPServer
	readOnly  *ReadOnlyMode
	logger    *slog.Logger
}

type ServerOpts struct {
	Logger               *slog.Logger
	ReadOnly             bool
	SlowRequestThreshold time.Duration
	SlowRequestLogLevel  slog.Level
}

func NewServer(opts ServerOpts) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	readOnly := NewReadOnlyMode(opts.ReadOnly)

	mcpOpts := []server.ServerOption{
		server.WithResourceCapabilities(true, false),
		server.WithPromptCapabilities(false),
	}

	if opts.SlowRequestThreshold > 0 {
		mcpOpts = append(mcpOpts, server.WithToolHandlerMiddleware(
			slowRequestMiddleware(opts.SlowRequestThreshold, opts.SlowRequestLogLevel, opts.Logger),
		))
	}

	mcpServer := server.NewMCPServer(
		"grafana-oncall-mcp",
		"0.1.0",
		mcpOpts...,
	)

	return &Server{
		mcpServer: mcpServer,
		readOnly:  readOnly,
		logger:    opts.Logger,
	}
}

func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}

func (s *Server) ReadOnly() *ReadOnlyMode {
	return s.readOnly
}

func (s *Server) Logger() *slog.Logger {
	return s.logger
}

func (s *Server) RegisterTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	if !s.readOnly.ShouldRegisterTool(tool.Name) {
		s.logger.Info("skipping write tool in read-only mode", "tool", tool.Name)
		return
	}
	s.mcpServer.AddTool(tool, handler)
}

func slowRequestMiddleware(threshold time.Duration, level slog.Level, logger *slog.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			result, err := next(ctx, req)
			elapsed := time.Since(start)
			if elapsed > threshold {
				logger.Log(ctx, level,
					"slow request",
					"tool", req.Params.Name,
					"duration_ms", elapsed.Milliseconds(),
				)
			}
			return result, err
		}
	}
}
