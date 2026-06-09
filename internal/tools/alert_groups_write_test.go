package tools

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	aapi "github.com/grafana/amixr-api-go-client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestBuildWriteResultAcknowledged(t *testing.T) {
	result := buildWriteResult("ag-1", "acknowledged", 200)
	assert.Equal(t, "ag-1", result.AlertGroupID)
	assert.Equal(t, oncall.AlertGroupStateAcknowledged, result.NewState)
	assert.True(t, result.WasAlreadyInState)
	assert.Equal(t, "service-account", result.ActingUser.Username)
	assert.False(t, result.PerformedAt.IsZero())
}

func TestBuildWriteResultResolved(t *testing.T) {
	result := buildWriteResult("ag-2", "resolved", 201)
	assert.Equal(t, "ag-2", result.AlertGroupID)
	assert.Equal(t, oncall.AlertGroupStateResolved, result.NewState)
	assert.True(t, result.WasAlreadyInState)
}

func TestBuildWriteResultFiring(t *testing.T) {
	result := buildWriteResult("ag-3", "new", 200)
	assert.Equal(t, oncall.AlertGroupStateNew, result.NewState)
	assert.True(t, result.WasAlreadyInState)
}

func TestUpstreamErrorMessageUsesRawBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test/api/v1/alert_groups/", nil)
	assert.NoError(t, err)

	errResp := &aapi.ErrorResponse{
		Body: []byte("missing required permission"),
		Response: &http.Response{
			StatusCode: http.StatusBadRequest,
			Request:    req,
		},
		Message: "failed to parse unknown error format",
	}

	assert.Equal(t, "upstream 400: missing required permission", upstreamErrorMessage(errResp))
}

func TestUpstreamUserNotFoundMapsToForbiddenWithHint(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test/api/v1/alert_groups/", nil)
	assert.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Request:    req,
	}
	errResp := &aapi.ErrorResponse{
		Body:     []byte("user sa-1-mcp-server not found"),
		Response: resp,
	}

	result, err := mapUpstreamError("list_alert_groups", errResp, resp)
	assert.NoError(t, err)
	assert.True(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var envelope oncall.Error
	assert.NoError(t, json.Unmarshal([]byte(text), &envelope))
	assert.Equal(t, oncall.ErrCodeForbidden, envelope.Code)
	assert.Contains(t, envelope.Message, "user sa-1-mcp-server not found")
	assert.NotNil(t, envelope.Hint)
	assert.Contains(t, *envelope.Hint, "GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm")
}

func TestSilenceResultDTO(t *testing.T) {
	performedAt := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	silenceUntil := performedAt.Add(30 * time.Minute)

	result := oncall.SilenceResult{
		WriteResult: oncall.WriteResult{
			AlertGroupID:      "ag-4",
			NewState:          oncall.AlertGroupStateSilenced,
			WasAlreadyInState: false,
			ActingUser: oncall.UserSummary{
				ID:       "service-account",
				Username: "service-account",
			},
			PerformedAt: performedAt,
		},
		SilencedUntil: silenceUntil,
	}

	assert.Equal(t, "ag-4", result.AlertGroupID)
	assert.Equal(t, oncall.AlertGroupStateSilenced, result.NewState)
	assert.False(t, result.WasAlreadyInState)
	assert.Equal(t, "service-account", result.ActingUser.Username)
	assert.Equal(t, silenceUntil.Unix(), result.SilencedUntil.Unix())
}

func TestAlertGroupStateTransitionRejected(t *testing.T) {
	err := oncall.NewStateTransitionRejectedError("acknowledge_alert_group", "resolved groups cannot be acknowledged")
	assert.Equal(t, oncall.ErrCodeStateTransitionRejected, err.Code)
	assert.False(t, err.Retryable)
	assert.Contains(t, err.Message, "resolved groups cannot be acknowledged")
}

func TestUpstreamErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
	}{
		{"401 returns error result", http.StatusUnauthorized},
		{"403 returns error result", http.StatusForbidden},
		{"404 returns error result", http.StatusNotFound},
		{"429 returns error result", http.StatusTooManyRequests},
		{"400 returns error result", http.StatusBadRequest},
		{"500 returns error result", http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.statusCode}
			result, err := mapUpstreamError("test_tool", assert.AnError, resp)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.True(t, result.IsError)
		})
	}
}
