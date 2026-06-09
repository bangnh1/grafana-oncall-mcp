package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestGetCurrentOncallUsersNotFoundIncludesScheduleHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/on_call_shifts/current_users/":
			assert.Equal(t, "sched-missing", r.URL.Query().Get("schedule_id"))
			http.Error(w, "not found", http.StatusNotFound)
		case "/api/v1/schedules/sched-missing/":
			http.Error(w, "not found", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := oncall.NewClientWithOptions("", "", oncall.ClientOptions{
		DirectAPIURL: server.URL,
		DirectAPIKey: "oncall-key",
	})
	assert.NoError(t, err)

	result, err := handleGetCurrentOncallUsers(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"schedule_id": "sched-missing"},
		},
	}, client)

	assert.NoError(t, err)
	assert.True(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var envelope oncall.Error
	assert.NoError(t, json.Unmarshal([]byte(text), &envelope))
	assert.Equal(t, oncall.ErrCodeNotFound, envelope.Code)
	assert.Contains(t, envelope.Message, "sched-missing")
	assert.Contains(t, envelope.Message, "upstream 404")
	assert.NotNil(t, envelope.Hint)
	assert.Contains(t, *envelope.Hint, "list_oncall_schedules")
}

func TestParseLimitConsistent(t *testing.T) {
	cases := []struct {
		input  any
		expect int
	}{
		{nil, 50},
		{float64(10), 10},
		{float64(0), 50},
		{float64(200), 200},
		{float64(500), 200},
		{"25", 25},
		{"abc", 50},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tc.expect, parseLimit(tc.input))
		})
	}
}

func TestBuildCursor(t *testing.T) {
	cursor, err := buildCursor(strPtr("http://example.com/api?page=2"), 50)
	assert.NoError(t, err)
	assert.NotNil(t, cursor)
	assert.Contains(t, *cursor, "page=2")

	cursor, err = buildCursor(nil, 50)
	assert.NoError(t, err)
	assert.Nil(t, cursor)

	cursor, err = buildCursor(strPtr(""), 50)
	assert.NoError(t, err)
	assert.Nil(t, cursor)
}
