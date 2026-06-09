package tools

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	aapi "github.com/grafana/amixr-api-go-client"
	"github.com/mark3labs/mcp-go/mcp"
)

func RegisterUserTools(srv ToolRegistry, client *oncall.Client) {
	registerGetCurrentOncallUsers(srv, client)
	registerListOncallUsers(srv, client)
}

func registerGetCurrentOncallUsers(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("get_current_oncall_users",
		mcp.WithDescription("Get the users currently on call for a schedule."),
		mcp.WithString("schedule_id",
			mcp.Description("Identifier of the schedule."),
			mcp.Required(),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetCurrentOncallUsers(ctx, req, client)
	})
}

func handleGetCurrentOncallUsers(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(&oncall.Error{
			Code:    oncall.ErrCodeInvalidInput,
			Message: "missing required argument: schedule_id",
			Tool:    "get_current_oncall_users",
		})
	}
	scheduleID, ok := args["schedule_id"].(string)
	if !ok || scheduleID == "" {
		return newErrorResult(&oncall.Error{
			Code:    oncall.ErrCodeInvalidInput,
			Message: "schedule_id must be a non-empty string",
			Tool:    "get_current_oncall_users",
		})
	}

	users, shiftEnd, httpResp, err := client.GetCurrentOnCallUsers(context.Background(), scheduleID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			return newErrorResult(oncall.WithHint(
				oncall.NewNotFoundError("get_current_oncall_users", fmt.Sprintf("schedule %q not found or current-users endpoint unavailable: %s", scheduleID, upstreamErrorMessage(err))),
				"verify schedule_id with list_oncall_schedules; if it exists, check the selected OnCall plugin/API URL supports /api/v1/on_call_shifts/current_users/",
			))
		}
		return mapUpstreamError("get_current_oncall_users", err, httpResp)
	}

	if users == nil {
		users = []oncall.UserSummary{}
	}

	result := oncall.CurrentOnCallResult{
		ScheduleID:  scheduleID,
		AsOf:        time.Now().UTC(),
		Users:       users,
		ShiftEndAt:  shiftEnd,
	}
	return jsonResult("get_current_oncall_users", result)
}

func registerListOncallUsers(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("list_oncall_users",
		mcp.WithDescription("List users in the Grafana OnCall organization, optionally filtered by exact username."),
		mcp.WithString("username",
			mcp.Description("Exact-match username filter. Returns at most one user when set."),
		),
		mcp.WithString("cursor",
			mcp.Description("Pagination continuation token from a previous call."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return (default 50, max 200)."),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListOncallUsers(ctx, req, client)
	})
}

func handleListOncallUsers(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		args = map[string]any{}
	}

	username, _ := args["username"].(string)
	limit := parseLimit(args["limit"])
	cursor, page := parseCursor(args["cursor"])

	opts := &aapi.ListUserOptions{
		ListOptions: aapi.ListOptions{Page: page},
	}
	if username != "" {
		opts.Username = username
	}

	if cursor != "" {
		nextURL, err := decodeCursor(cursor)
		if err != nil {
			return newErrorResult(&oncall.Error{
				Code:    oncall.ErrCodeInvalidInput,
				Message: fmt.Sprintf("invalid cursor: %v", err),
				Tool:    "list_oncall_users",
			})
		}
		opts.Page = extractPageFromURL(nextURL, page)
	}

	rawResp, httpResp, err := client.Users().ListUsers(opts)
	if err != nil {
		return mapUpstreamError("list_oncall_users", err, httpResp)
	}

	items := make([]oncall.User, 0, len(rawResp.Users))
	for _, u := range rawResp.Users {
		item := oncall.User{
			ID:       u.ID,
			Username: u.Username,
		}
		if u.Email != "" {
			item.Email = strPtr(u.Email)
		}
		if u.Role != "" {
			item.Role = strPtr(u.Role)
		}
		items = append(items, item)
	}

	nextCursor, _ := buildCursor(rawResp.Next, limit)
	result := oncall.Page[oncall.User]{
		Items:         items,
		NextCursor:    nextCursor,
		TotalEstimate: intPtr(rawResp.Count),
	}
	return jsonResult("list_oncall_users", result)
}
