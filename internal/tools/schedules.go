package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	aapi "github.com/grafana/amixr-api-go-client"
	"github.com/mark3labs/mcp-go/mcp"
)

func RegisterScheduleTools(srv ToolRegistry, client *oncall.Client) {
	registerListOncallSchedules(srv, client)
	registerGetOncallShift(srv, client)
}

func registerListOncallSchedules(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("list_oncall_schedules",
		mcp.WithDescription("List on-call schedules, optionally filtered by team."),
		mcp.WithString("team_id",
			mcp.Description("If set, only schedules owned by this team are returned."),
		),
		mcp.WithString("cursor",
			mcp.Description("Pagination continuation token from a previous call."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return (default 50, max 200)."),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListOncallSchedules(ctx, req, client)
	})
}

func handleListOncallSchedules(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		args = map[string]any{}
	}

	teamID, _ := args["team_id"].(string)
	limit := parseLimit(args["limit"])
	cursor, page := parseCursor(args["cursor"])

	amixrOpts := &aapi.ListScheduleOptions{
		TeamID: teamID,
		ListOptions: aapi.ListOptions{
			Page: page,
		},
	}

	if cursor != "" {
		nextURL, err := decodeCursor(cursor)
		if err != nil {
			return newErrorResult(&oncall.Error{
				Code:    oncall.ErrCodeInvalidInput,
				Message: fmt.Sprintf("invalid cursor: %v", err),
				Tool:    "list_oncall_schedules",
			})
		}
		amixrOpts.Page = extractPageFromURL(nextURL, page)
	}

	rawResp, httpResp, err := client.Schedules().ListSchedules(amixrOpts)
	if err != nil {
		return mapUpstreamError("list_oncall_schedules", err, httpResp)
	}

	items := make([]oncall.Schedule, 0, len(rawResp.Schedules))
	for _, s := range rawResp.Schedules {
		tz := s.TimeZone
		if tz == "" {
			tz = "UTC"
		}
		shiftIDs := []string{}
		if s.Shifts != nil {
			shiftIDs = *s.Shifts
		}
		items = append(items, oncall.Schedule{
			ID:                  s.ID,
			Name:                s.Name,
			TeamID:              strPtr(s.TeamId),
			Type:                oncall.ScheduleType(s.Type),
			Timezone:            tz,
			TimezoneWasInferred: false,
			ShiftIDs:            shiftIDs,
		})
	}

	nextCursor, _ := buildCursor(rawResp.Next, limit)
	result := oncall.Page[oncall.Schedule]{
		Items:         items,
		NextCursor:    nextCursor,
		TotalEstimate: intPtr(rawResp.Count),
	}

	return jsonResult("list_oncall_schedules", result)
}

func registerGetOncallShift(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("get_oncall_shift",
		mcp.WithDescription("Retrieve a single on-call shift by ID."),
		mcp.WithString("shift_id",
			mcp.Description("Identifier of the shift to retrieve."),
			mcp.Required(),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetOncallShift(ctx, req, client)
	})
}

func handleGetOncallShift(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(&oncall.Error{
			Code:    oncall.ErrCodeInvalidInput,
			Message: "missing required argument: shift_id",
			Tool:    "get_oncall_shift",
		})
	}

	shiftID, ok := args["shift_id"].(string)
	if !ok || shiftID == "" {
		return newErrorResult(&oncall.Error{
			Code:    oncall.ErrCodeInvalidInput,
			Message: "shift_id must be a non-empty string",
			Tool:    "get_oncall_shift",
		})
	}

	rawShift, httpResp, err := client.Shifts().GetOnCallShift(shiftID, &aapi.GetOnCallShiftOptions{})
	if err != nil {
		return mapUpstreamError("get_oncall_shift", err, httpResp)
	}

	start, _ := time.Parse(time.RFC3339, rawShift.Start)
	dur := time.Duration(rawShift.Duration) * time.Second

	users := make([]oncall.UserSummary, 0)
	if rawShift.Users != nil {
		users = make([]oncall.UserSummary, len(*rawShift.Users))
		for i, uid := range *rawShift.Users {
			users[i] = oncall.UserSummary{ID: uid, Username: uid}
		}
	}

	var endAt *time.Time
	if rawShift.Until != nil {
		t, err := time.Parse(time.RFC3339, *rawShift.Until)
		if err == nil {
			endAt = &t
		}
	}

	var rotationStart *time.Time
	if rawShift.TimeZone != nil {
		rotationStart = &start
	}

	var rollingUsers [][]string
	if rawShift.RollingUsers != nil {
		rollingUsers = make([][]string, len(*rawShift.RollingUsers))
		copy(rollingUsers, *rawShift.RollingUsers)
	}

	shift := oncall.Shift{
		ID:           rawShift.ID,
		ScheduleID:   rawShift.TeamId,
		Type:         oncall.ShiftType(rawShift.Type),
		StartAt:      start,
		EndAt:        endAt,
		Duration:     dur.String(),
		Users:        users,
		Level:        rawShift.Level,
		TimeZone:     rawShift.TimeZone,
		RollingUsers: rollingUsers,
	}

	if rawShift.Name != "" {
		shift.Name = &rawShift.Name
	}
	if rawShift.Frequency != nil {
		f := oncall.ShiftFrequency(*rawShift.Frequency)
		shift.Frequency = (*string)(&f)
	}
	if rawShift.Interval != nil {
		ival := *rawShift.Interval
		shift.Interval = &ival
	}
	if rawShift.WeekStart != nil {
		shift.WeekStart = rawShift.WeekStart
	}
	if rawShift.ByDay != nil {
		shift.ByDay = make([]string, len(*rawShift.ByDay))
		copy(shift.ByDay, *rawShift.ByDay)
	}
	if rawShift.ByMonth != nil {
		shift.ByMonth = make([]int, len(*rawShift.ByMonth))
		copy(shift.ByMonth, *rawShift.ByMonth)
	}
	if rawShift.ByMonthday != nil {
		shift.ByMonthday = make([]int, len(*rawShift.ByMonthday))
		copy(shift.ByMonthday, *rawShift.ByMonthday)
	}
	if rawShift.StartRotationFromUserIndex != nil {
		shift.StartRotationFromUserIndex = rawShift.StartRotationFromUserIndex
	}

	shift.RotationStart = rotationStart

	return jsonResult("get_oncall_shift", shift)
}

// --- helpers ---

func parseLimit(v any) int {
	switch val := v.(type) {
	case float64:
		if val <= 0 {
			return 50
		}
		if val > 200 {
			return 200
		}
		return int(val)
	case int:
		if val <= 0 {
			return 50
		}
		if val > 200 {
			return 200
		}
		return val
	case string:
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return 50
		}
		if n > 200 {
			return 200
		}
		return n
	default:
		return 50
	}
}

func parseCursor(v any) (string, int) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return "", 1
		}
		decoded, err := decodeCursor(val)
		if err != nil {
			return "", 1
		}
		page := extractPageFromURL(decoded, 1)
		return val, page
	default:
		return "", 1
	}
}

func decodeCursor(cursor string) (string, error) {
	data, err := strconv.Unquote(cursor)
	if err != nil {
		return "", fmt.Errorf("unquote cursor: %w", err)
	}
	return data, nil
}

func extractPageFromURL(nextURL string, defaultPage int) int {
	if nextURL == "" {
		return defaultPage
	}
	u, err := url.Parse(nextURL)
	if err != nil {
		return defaultPage
	}
	pageStr := u.Query().Get("page")
	if pageStr == "" {
		return defaultPage
	}
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		return defaultPage
	}
	return page + 1
}

func buildCursor(nextURL *string, limit int) (*string, error) {
	if nextURL == nil || *nextURL == "" {
		return nil, nil
	}
	encoded := strconv.Quote(*nextURL)
	return &encoded, nil
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func newErrorResult(err *oncall.Error) (*mcp.CallToolResult, error) {
	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: string(data)},
		},
		IsError: true,
	}, nil
}

func jsonResult(tool string, v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return newErrorResult(oncall.NewInternalError(tool, fmt.Sprintf("marshal response: %v", err)))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: string(data)},
		},
	}, nil
}

func mapUpstreamError(tool string, err error, httpResp *http.Response) (*mcp.CallToolResult, error) {
	message := upstreamErrorMessage(err)
	oncallErr := oncall.NewUpstreamUnavailableError(tool, message)
	if httpResp != nil {
		switch httpResp.StatusCode {
		case 401:
			oncallErr = oncall.NewUnauthenticatedError(tool, "invalid or missing Grafana token")
		case 403:
			oncallErr = oncall.NewForbiddenError(tool, "insufficient permissions for this operation")
		case 404:
			oncallErr = oncall.NewNotFoundError(tool, "requested resource not found")
		case 429:
			oncallErr = oncall.NewUpstreamRateLimitedError(tool, "upstream rate limit exceeded")
		case 400:
			if isOnCallUserNotFoundError(message) {
				oncallErr = oncall.WithHint(
					oncall.NewForbiddenError(tool, message),
					"grafana-oncall-app resolved the token to a Grafana service account that is not an OnCall user; set GRAFANA_ONCALL_API_URL and GRAFANA_ONCALL_API_KEY for direct OnCall API mode, or set GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm if grafana-irm-app is installed",
				)
			} else {
				oncallErr = oncall.NewInvalidInputError(tool, message)
			}
		}
	}
	return newErrorResult(oncallErr)
}

func isOnCallUserNotFoundError(message string) bool {
	normalized := strings.ToLower(message)
	return strings.Contains(normalized, "user sa-") && strings.Contains(normalized, "not found")
}

func upstreamErrorMessage(err error) string {
	var upstreamErr *aapi.ErrorResponse
	if errors.As(err, &upstreamErr) {
		body := strings.TrimSpace(string(upstreamErr.Body))
		if body != "" {
			if len(body) > 1024 {
				body = body[:1024] + "..."
			}
			return fmt.Sprintf("upstream %d: %s", upstreamErr.Response.StatusCode, body)
		}
		if upstreamErr.Message != "" {
			return fmt.Sprintf("upstream %d: %s", upstreamErr.Response.StatusCode, upstreamErr.Message)
		}
	}
	return err.Error()
}
