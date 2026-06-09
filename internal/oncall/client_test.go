package oncall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClientWithDirectOnCallAPI(t *testing.T) {
	client, err := NewClientWithOptions("", "", ClientOptions{
		HTTPTimeout:  2 * time.Second,
		DirectAPIURL: "https://oncall.example.com",
		DirectAPIKey: "oncall-key",
	})

	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, PluginDirectAPI, client.Plugin())
}

func TestNewClientWithDirectOnCallAPIRequiresURLAndKey(t *testing.T) {
	client, err := NewClientWithOptions("", "", ClientOptions{
		DirectAPIURL: "https://oncall.example.com",
	})

	assert.Nil(t, client)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "direct OnCall API requires both URL and API key")
}

func TestGetCurrentOnCallUsersEncodesScheduleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/on_call_shifts/current_users/", r.URL.Path)
		assert.Equal(t, "sched-1", r.URL.Query().Get("schedule_id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"user":{"id":"user-1","username":"alice"},"shift_end_at":"2026-06-09T10:00:00Z"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClientWithOptions("", "", ClientOptions{
		DirectAPIURL: server.URL,
		DirectAPIKey: "oncall-key",
	})
	assert.NoError(t, err)

	users, shiftEnd, resp, err := client.GetCurrentOnCallUsers(context.Background(), "sched-1")

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotNil(t, shiftEnd)
	assert.Equal(t, []UserSummary{{ID: "user-1", Username: "alice"}}, users)
}

func TestGetCurrentOnCallUsersFallsBackToScheduleOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/on_call_shifts/current_users/":
			assert.Equal(t, "sched-1", r.URL.Query().Get("schedule_id"))
			http.Error(w, `{"detail":"Not found."}`, http.StatusNotFound)
		case "/api/v1/schedules/sched-1/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sched-1","name":"Primary","on_call_now":["user-1","user-2"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClientWithOptions("", "", ClientOptions{
		DirectAPIURL: server.URL,
		DirectAPIKey: "oncall-key",
	})
	assert.NoError(t, err)

	users, shiftEnd, resp, err := client.GetCurrentOnCallUsers(context.Background(), "sched-1")

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, shiftEnd)
	assert.Equal(t, []UserSummary{
		{ID: "user-1", Username: "user-1"},
		{ID: "user-2", Username: "user-2"},
	}, users)
}
