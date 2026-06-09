package tools

import (
	"context"
	"fmt"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	aapi "github.com/grafana/amixr-api-go-client"
	"github.com/mark3labs/mcp-go/mcp"
)

func RegisterTeamTools(srv ToolRegistry, client *oncall.Client) {
	registerListOncallTeams(srv, client)
}

func registerListOncallTeams(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("list_oncall_teams",
		mcp.WithDescription("List teams in the Grafana OnCall organization."),
		mcp.WithString("cursor",
			mcp.Description("Pagination continuation token from a previous call."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return (default 50, max 200)."),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListOncallTeams(ctx, req, client)
	})
}

func handleListOncallTeams(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		args = map[string]any{}
	}

	limit := parseLimit(args["limit"])
	cursor, page := parseCursor(args["cursor"])

	opts := &aapi.ListTeamOptions{
		ListOptions: aapi.ListOptions{Page: page},
	}
	if cursor != "" {
		nextURL, err := decodeCursor(cursor)
		if err != nil {
			return newErrorResult(&oncall.Error{
				Code:    oncall.ErrCodeInvalidInput,
				Message: fmt.Sprintf("invalid cursor: %v", err),
				Tool:    "list_oncall_teams",
			})
		}
		opts.Page = extractPageFromURL(nextURL, page)
	}

	rawResp, httpResp, err := client.Teams().ListTeams(opts)
	if err != nil {
		return mapUpstreamError("list_oncall_teams", err, httpResp)
	}

	items := make([]oncall.Team, 0, len(rawResp.Teams))
	for _, t := range rawResp.Teams {
		tm := oncall.Team{
			ID:   t.ID,
			Name: t.Name,
		}
		if t.Email != "" {
			tm.Email = strPtr(t.Email)
		}
		if t.AvatarUrl != "" {
			tm.AvatarURL = strPtr(t.AvatarUrl)
		}
		items = append(items, tm)
	}

	nextCursor, _ := buildCursor(rawResp.Next, limit)
	result := oncall.Page[oncall.Team]{
		Items:         items,
		NextCursor:    nextCursor,
		TotalEstimate: intPtr(rawResp.Count),
	}
	return jsonResult("list_oncall_teams", result)
}
