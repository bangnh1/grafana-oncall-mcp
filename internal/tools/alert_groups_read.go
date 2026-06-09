package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	aapi "github.com/grafana/amixr-api-go-client"
	"github.com/mark3labs/mcp-go/mcp"
)

func RegisterAlertGroupReadTools(srv ToolRegistry, client *oncall.Client) {
	registerListAlertGroups(srv, client)
	registerGetAlertGroup(srv, client)
}

func registerListAlertGroups(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("list_alert_groups",
		mcp.WithDescription("List alert groups with filtering by state, integration, route, team, labels, time range, and free-text search."),
		mcp.WithString("state",
			mcp.Description("Filter by current state: new, acknowledged, resolved, or silenced. The legacy alias firing is accepted and sent as new."),
		),
		mcp.WithString("integration_id",
			mcp.Description("Filter by originating integration."),
		),
		mcp.WithString("route_id",
			mcp.Description("Filter by matched route."),
		),
		mcp.WithString("team_id",
			mcp.Description("Filter by owning team."),
		),
		mcp.WithObject("labels",
			mcp.Description("AND-combined label match (e.g. {\"severity\":\"critical\"})."),
		),
		mcp.WithString("started_at_from",
			mcp.Description("ISO 8601 UTC start of the time range."),
		),
		mcp.WithString("started_at_to",
			mcp.Description("ISO 8601 UTC end of the time range."),
		),
		mcp.WithString("search",
			mcp.Description("Free-text search across title and labels."),
		),
		mcp.WithString("cursor",
			mcp.Description("Pagination continuation token from a previous call."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of items to return (default 50, max 200)."),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListAlertGroups(ctx, req, client)
	})
}

func handleListAlertGroups(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		args = map[string]any{}
	}

	limit := parseLimit(args["limit"])
	cursor, page := parseCursor(args["cursor"])

	opts := &aapi.ListAlertGroupOptions{
		ListOptions: aapi.ListOptions{Page: page},
	}

	if state, ok := args["state"].(string); ok && state != "" {
		normalized, err := normalizeAlertGroupStateFilter(state)
		if err != nil {
			return newErrorResult(oncall.NewInvalidInputError("list_alert_groups", err.Error()))
		}
		opts.State = normalized
	}
	if intID, ok := args["integration_id"].(string); ok && intID != "" {
		opts.IntegrationID = intID
	}
	if routeID, ok := args["route_id"].(string); ok && routeID != "" {
		opts.RouteID = routeID
	}
	if teamID, ok := args["team_id"].(string); ok && teamID != "" {
		opts.TeamID = teamID
	}
	if labels, ok := args["labels"].(map[string]any); ok && len(labels) > 0 {
		opts.Labels = mapToLabelPairs(labels)
	}
	if search, ok := args["search"].(string); ok && search != "" {
		opts.Name = search
	}

	startedAtFrom, _ := args["started_at_from"].(string)
	startedAtTo, _ := args["started_at_to"].(string)
	if startedAtFrom != "" || startedAtTo != "" {
		startedAt, err := buildStartedAtRange(startedAtFrom, startedAtTo)
		if err != nil {
			return newErrorResult(oncall.NewInvalidInputError("list_alert_groups", err.Error()))
		}
		opts.StartedAt = startedAt
	}

	if cursor != "" {
		nextURL, err := decodeCursor(cursor)
		if err != nil {
			return newErrorResult(&oncall.Error{
				Code:    oncall.ErrCodeInvalidInput,
				Message: fmt.Sprintf("invalid cursor: %v", err),
				Tool:    "list_alert_groups",
			})
		}
		opts.Page = extractPageFromURL(nextURL, page)
	}

	rawResp, httpResp, err := client.AlertGroups().ListAlertGroups(opts)
	if err != nil {
		return mapUpstreamError("list_alert_groups", err, httpResp)
	}

	items := make([]oncall.AlertGroupSummary, 0, len(rawResp.AlertGroups))
	for _, ag := range rawResp.AlertGroups {
		items = append(items, mapAlertGroupToSummary(ag))
	}

	nextCursor, _ := buildCursor(rawResp.Next, limit)
	result := oncall.Page[oncall.AlertGroupSummary]{
		Items:         items,
		NextCursor:    nextCursor,
		TotalEstimate: intPtr(rawResp.Count),
	}

	return jsonResult("list_alert_groups", result)
}

func registerGetAlertGroup(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("get_alert_group",
		mcp.WithDescription("Retrieve a single alert group by ID with full details including labels and permalink."),
		mcp.WithString("alert_group_id",
			mcp.Description("Identifier of the alert group to retrieve."),
			mcp.Required(),
		),
	)

	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleGetAlertGroup(ctx, req, client)
	})
}

func handleGetAlertGroup(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(&oncall.Error{
			Code:    oncall.ErrCodeInvalidInput,
			Message: "missing required argument: alert_group_id",
			Tool:    "get_alert_group",
		})
	}

	alertGroupID, ok := args["alert_group_id"].(string)
	if !ok || alertGroupID == "" {
		return newErrorResult(&oncall.Error{
			Code:    oncall.ErrCodeInvalidInput,
			Message: "alert_group_id must be a non-empty string",
			Tool:    "get_alert_group",
		})
	}

	rawAG, httpResp, err := client.AlertGroups().GetAlertGroup(alertGroupID)
	if err != nil {
		return mapUpstreamError("get_alert_group", err, httpResp)
	}

	result := mapAlertGroupToFull(rawAG)
	return jsonResult("get_alert_group", result)
}

func mapAlertGroupToSummary(ag *aapi.AlertGroup) oncall.AlertGroupSummary {
	createdAt, _ := time.Parse(time.RFC3339, ag.CreatedAt)
	updatedAt := createdAt
	if ag.ResolvedAt != "" {
		t, err := time.Parse(time.RFC3339, ag.ResolvedAt)
		if err == nil {
			updatedAt = t
		}
	}
	if ag.AcknowledgedAt != "" {
		t, err := time.Parse(time.RFC3339, ag.AcknowledgedAt)
		if err == nil && t.After(updatedAt) {
			updatedAt = t
		}
	}

	summary := oncall.AlertGroupSummary{
		ID:              ag.ID,
		Title:           ag.Title,
		State:           oncall.AlertGroupState(ag.State),
		IntegrationID:   ag.IntegrationID,
		IntegrationName: ag.IntegrationID,
		RouteID:         strOrNil(ag.RouteID),
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		AlertsCount:     ag.AlertsCount,
	}

	if ag.ResolvedAt != "" {
		t, err := time.Parse(time.RFC3339, ag.ResolvedAt)
		if err == nil {
			summary.ResolvedAt = &t
		}
	}
	if ag.AcknowledgedAt != "" {
		t, err := time.Parse(time.RFC3339, ag.AcknowledgedAt)
		if err == nil {
			summary.AcknowledgedAt = &t
		}
	}

	return summary
}

func mapAlertGroupToFull(ag *aapi.AlertGroup) oncall.AlertGroup {
	summary := mapAlertGroupToSummary(ag)

	labels := oncall.StringMap{}
	permalink := ""
	if len(ag.Permalinks) > 0 {
		for _, v := range ag.Permalinks {
			permalink = v
			break
		}
	}

	return oncall.AlertGroup{
		AlertGroupSummary: summary,
		Labels:            labels,
		Permalink:         permalink,
	}
}

func mapToLabelPairs(labels map[string]any) []string {
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		val, ok := v.(string)
		if !ok {
			val = fmt.Sprintf("%v", v)
		}
		pairs = append(pairs, fmt.Sprintf("%s:%s", k, val))
	}
	sort.Strings(pairs)
	return pairs
}

func normalizeAlertGroupStateFilter(state string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(state)) {
	case "new", "firing":
		return "new", nil
	case "acknowledged", "resolved", "silenced":
		return strings.TrimSpace(strings.ToLower(state)), nil
	default:
		return "", fmt.Errorf("state must be one of: new, acknowledged, resolved, silenced")
	}
}

func buildStartedAtRange(from, to string) (string, error) {
	if from == "" && to == "" {
		return "", nil
	}

	now := time.Now().UTC()
	defaultFrom := now.Add(-24 * time.Hour)
	defaultTo := now

	var startTime, endTime time.Time
	var err error

	if from != "" {
		startTime, err = time.Parse(time.RFC3339, from)
		if err != nil {
			return "", fmt.Errorf("started_at_from must be valid ISO 8601 UTC: %w", err)
		}
	} else {
		startTime = defaultFrom
	}

	if to != "" {
		endTime, err = time.Parse(time.RFC3339, to)
		if err != nil {
			return "", fmt.Errorf("started_at_to must be valid ISO 8601 UTC: %w", err)
		}
	} else {
		endTime = defaultTo
	}

	if endTime.Before(startTime) {
		return "", fmt.Errorf("started_at_to must be after started_at_from")
	}

	return fmt.Sprintf("%s_%s",
		startTime.Format("2006-01-02T15:04:05"),
		endTime.Format("2006-01-02T15:04:05"),
	), nil
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
