package tools

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	"github.com/mark3labs/mcp-go/mcp"
)

func RegisterAlertGroupWriteTools(srv ToolRegistry, client *oncall.Client) {
	registerAcknowledgeAlertGroup(srv, client)
	registerResolveAlertGroup(srv, client)
	registerSilenceAlertGroup(srv, client)
	registerUnresolveAlertGroup(srv, client)
}

func registerAcknowledgeAlertGroup(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("acknowledge_alert_group",
		mcp.WithDescription("Acknowledge an alert group."),
		mcp.WithString("alert_group_id",
			mcp.Description("Identifier of the alert group to acknowledge."),
			mcp.Required(),
		),
	)
	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleAcknowledgeAlertGroup(ctx, req, client)
	})
}

func handleAcknowledgeAlertGroup(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(oncall.NewInvalidInputError("acknowledge_alert_group", "missing required argument: alert_group_id"))
	}
	id, ok := args["alert_group_id"].(string)
	if !ok || id == "" {
		return newErrorResult(oncall.NewInvalidInputError("acknowledge_alert_group", "alert_group_id must be a non-empty string"))
	}
	return performWrite2("acknowledge_alert_group", client.AcknowledgeAlertGroup, id, "acknowledged")
}

func registerResolveAlertGroup(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("resolve_alert_group",
		mcp.WithDescription("Resolve an alert group."),
		mcp.WithString("alert_group_id",
			mcp.Description("Identifier of the alert group to resolve."),
			mcp.Required(),
		),
	)
	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleResolveAlertGroup(ctx, req, client)
	})
}

func handleResolveAlertGroup(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(oncall.NewInvalidInputError("resolve_alert_group", "missing required argument: alert_group_id"))
	}
	id, ok := args["alert_group_id"].(string)
	if !ok || id == "" {
		return newErrorResult(oncall.NewInvalidInputError("resolve_alert_group", "alert_group_id must be a non-empty string"))
	}
	return performWrite2("resolve_alert_group", client.ResolveAlertGroup, id, "resolved")
}

func registerSilenceAlertGroup(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("silence_alert_group",
		mcp.WithDescription("Silence an alert group for a specified duration."),
		mcp.WithString("alert_group_id",
			mcp.Description("Identifier of the alert group to silence."),
			mcp.Required(),
		),
		mcp.WithString("until",
			mcp.Description("Future timestamp (ISO 8601 UTC) when silence expires."),
			mcp.Required(),
		),
		mcp.WithString("comment",
			mcp.Description("Optional operator-visible reason for silencing."),
		),
	)
	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleSilenceAlertGroup(ctx, req, client)
	})
}

func handleSilenceAlertGroup(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(oncall.NewInvalidInputError("silence_alert_group", "missing required arguments: alert_group_id, until"))
	}
	id, ok := args["alert_group_id"].(string)
	if !ok || id == "" {
		return newErrorResult(oncall.NewInvalidInputError("silence_alert_group", "alert_group_id must be a non-empty string"))
	}
	untilStr, ok := args["until"].(string)
	if !ok || untilStr == "" {
		return newErrorResult(oncall.NewInvalidInputError("silence_alert_group", "until must be a non-empty ISO 8601 UTC timestamp"))
	}
	until, err := time.Parse(time.RFC3339, untilStr)
	if err != nil {
		return newErrorResult(oncall.NewInvalidInputError("silence_alert_group", fmt.Sprintf("until must be valid ISO 8601 UTC: %v", err)))
	}
	if until.Before(time.Now().UTC()) {
		return newErrorResult(oncall.NewInvalidInputError("silence_alert_group", "until must be in the future"))
	}

	delaySeconds := int(time.Until(until).Seconds())
	if delaySeconds <= 0 {
		delaySeconds = 1
	}

	return performWrite3("silence_alert_group", client.SilenceAlertGroup, id, delaySeconds, "silenced")
}

func registerUnresolveAlertGroup(srv ToolRegistry, client *oncall.Client) {
	tool := mcp.NewTool("unresolve_alert_group",
		mcp.WithDescription("Unresolve an alert group, returning it to new or acknowledged state."),
		mcp.WithString("alert_group_id",
			mcp.Description("Identifier of the alert group to unresolve."),
			mcp.Required(),
		),
	)
	srv.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleUnresolveAlertGroup(ctx, req, client)
	})
}

func handleUnresolveAlertGroup(_ context.Context, req mcp.CallToolRequest, client *oncall.Client) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	if args == nil {
		return newErrorResult(oncall.NewInvalidInputError("unresolve_alert_group", "missing required argument: alert_group_id"))
	}
	id, ok := args["alert_group_id"].(string)
	if !ok || id == "" {
		return newErrorResult(oncall.NewInvalidInputError("unresolve_alert_group", "alert_group_id must be a non-empty string"))
	}
	return performWrite2("unresolve_alert_group", client.UnresolveAlertGroup, id, string(oncall.AlertGroupStateNew))
}

// --- shared write helpers ---

type write2Func func(ctx context.Context, id string) (*http.Response, error)
type write3Func func(ctx context.Context, id string, delaySeconds int) (*http.Response, error)

func performWrite2(tool string, fn write2Func, id string, expectedState string) (*mcp.CallToolResult, error) {
	ctx := context.Background()
	resp, err := fn(ctx, id)
	if err != nil {
		message := upstreamErrorMessage(err)
		oncallErr := oncall.NewUpstreamUnavailableError(tool, message)
		if resp != nil {
			switch resp.StatusCode {
			case 401:
				oncallErr = oncall.NewUnauthenticatedError(tool, "invalid or missing Grafana token")
			case 403:
				oncallErr = oncall.NewForbiddenError(tool, "insufficient permissions")
			case 404:
				oncallErr = oncall.NewNotFoundError(tool, fmt.Sprintf("alert group %q not found", id))
			case 400:
				oncallErr = oncall.NewInvalidInputError(tool, message)
			case 409:
				oncallErr = oncall.NewStateTransitionRejectedError(tool, "invalid state transition for current alert group state")
			}
		}
		return newErrorResult(oncallErr)
	}
	defer resp.Body.Close()
	return jsonResult(tool, buildWriteResult(id, expectedState, resp.StatusCode))
}

func performWrite3(tool string, fn write3Func, id string, delaySeconds int, expectedState string) (*mcp.CallToolResult, error) {
	ctx := context.Background()
	resp, err := fn(ctx, id, delaySeconds)
	if err != nil {
		message := upstreamErrorMessage(err)
		oncallErr := oncall.NewUpstreamUnavailableError(tool, message)
		if resp != nil {
			switch resp.StatusCode {
			case 401:
				oncallErr = oncall.NewUnauthenticatedError(tool, "invalid or missing Grafana token")
			case 403:
				oncallErr = oncall.NewForbiddenError(tool, "insufficient permissions")
			case 404:
				oncallErr = oncall.NewNotFoundError(tool, fmt.Sprintf("alert group %q not found", id))
			case 400:
				oncallErr = oncall.NewInvalidInputError(tool, message)
			case 409:
				oncallErr = oncall.NewStateTransitionRejectedError(tool, "invalid state transition for current alert group state")
			}
		}
		return newErrorResult(oncallErr)
	}
	defer resp.Body.Close()

	performedAt := time.Now().UTC()
	silenceUntil := performedAt.Add(time.Duration(delaySeconds) * time.Second)

	result := oncall.SilenceResult{
		WriteResult: oncall.WriteResult{
			AlertGroupID:      id,
			NewState:          oncall.AlertGroupState(expectedState),
			WasAlreadyInState: resp.StatusCode == 200 || resp.StatusCode == 201,
			ActingUser: oncall.UserSummary{
				ID:       "service-account",
				Username: "service-account",
			},
			PerformedAt: performedAt,
		},
		SilencedUntil: silenceUntil,
	}

	return jsonResult(tool, result)
}

func buildWriteResult(id, expectedState string, statusCode int) oncall.WriteResult {
	performedAt := time.Now().UTC()
	return oncall.WriteResult{
		AlertGroupID:      id,
		NewState:          oncall.AlertGroupState(expectedState),
		WasAlreadyInState: statusCode == 200 || statusCode == 201,
		ActingUser: oncall.UserSummary{
			ID:       "service-account",
			Username: "service-account",
		},
		PerformedAt: performedAt,
	}
}
