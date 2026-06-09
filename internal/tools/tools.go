package tools

import (
	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ToolRegistry interface {
	RegisterTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}

func AddOnCallTools(srv ToolRegistry, client *oncall.Client) {
	RegisterScheduleTools(srv, client)
	RegisterTeamTools(srv, client)
	RegisterUserTools(srv, client)
	RegisterAlertGroupReadTools(srv, client)
	RegisterAlertGroupWriteTools(srv, client)
}
