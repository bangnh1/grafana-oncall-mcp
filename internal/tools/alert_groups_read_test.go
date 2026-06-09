package tools

import (
	"testing"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	"github.com/stretchr/testify/assert"
)

func TestAlertGroupSummaryDTOMapping(t *testing.T) {
	createdAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	resolvedAt := time.Date(2026, 6, 8, 11, 0, 0, 0, time.UTC)
	acknowledgedAt := time.Date(2026, 6, 8, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 8, 10, 30, 0, 0, time.UTC)

	ag := oncall.AlertGroupSummary{
		ID:              "ag-1",
		Title:           "High CPU on web-1",
		State:           oncall.AlertGroupStateAcknowledged,
		Severity:        strPtr("critical"),
		IntegrationID:   "int-1",
		IntegrationName: "Grafana Alerting",
		RouteID:         strPtr("route-1"),
		TeamID:          strPtr("team-1"),
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		ResolvedAt:      &resolvedAt,
		AcknowledgedAt:  &acknowledgedAt,
		AcknowledgedBy:  &oncall.UserSummary{ID: "user-1", Username: "alice"},
		SilencedUntil:   nil,
		AlertsCount:     3,
	}

	assert.Equal(t, "ag-1", ag.ID)
	assert.Equal(t, "High CPU on web-1", ag.Title)
	assert.Equal(t, oncall.AlertGroupStateAcknowledged, ag.State)
	assert.Equal(t, "critical", *ag.Severity)
	assert.Equal(t, "int-1", ag.IntegrationID)
	assert.Equal(t, "Grafana Alerting", ag.IntegrationName)
	assert.Equal(t, "route-1", *ag.RouteID)
	assert.Equal(t, "team-1", *ag.TeamID)
	assert.Equal(t, createdAt, ag.CreatedAt)
	assert.Equal(t, updatedAt, ag.UpdatedAt)
	assert.NotNil(t, ag.ResolvedAt)
	assert.NotNil(t, ag.AcknowledgedAt)
	assert.NotNil(t, ag.AcknowledgedBy)
	assert.Equal(t, "alice", ag.AcknowledgedBy.Username)
	assert.Nil(t, ag.SilencedUntil)
	assert.Equal(t, 3, ag.AlertsCount)
}

func TestAlertGroupFullDTOMapping(t *testing.T) {
	createdAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 8, 10, 30, 0, 0, time.UTC)

	ag := oncall.AlertGroup{
		AlertGroupSummary: oncall.AlertGroupSummary{
			ID:              "ag-2",
			Title:           "Disk full",
			State:           oncall.AlertGroupStateFiring,
			IntegrationID:   "int-2",
			IntegrationName: "Webhook",
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			AlertsCount:     2,
		},
		Labels:    oncall.StringMap{"severity": "critical", "env": "prod"},
		Permalink: "https://oncall.example.com/alert-groups/ag-2",
	}

	assert.Equal(t, "ag-2", ag.ID)
	assert.Equal(t, oncall.AlertGroupStateFiring, ag.State)
	assert.Len(t, ag.Labels, 2)
	assert.Equal(t, "critical", ag.Labels["severity"])
	assert.Equal(t, "prod", ag.Labels["env"])
	assert.Contains(t, ag.Permalink, "ag-2")
}

func TestAlertGroupStates(t *testing.T) {
	assert.Equal(t, oncall.AlertGroupState("new"), oncall.AlertGroupStateNew)
	assert.Equal(t, oncall.AlertGroupStateNew, oncall.AlertGroupStateFiring)
	assert.Equal(t, oncall.AlertGroupState("acknowledged"), oncall.AlertGroupStateAcknowledged)
	assert.Equal(t, oncall.AlertGroupState("resolved"), oncall.AlertGroupStateResolved)
	assert.Equal(t, oncall.AlertGroupState("silenced"), oncall.AlertGroupStateSilenced)
}

func TestNormalizeAlertGroupStateFilter(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "new", input: "new", want: "new"},
		{name: "firing alias", input: "firing", want: "new"},
		{name: "case and spaces", input: " Acknowledged ", want: "acknowledged"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeAlertGroupStateFilter(tc.input)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := normalizeAlertGroupStateFilter("unknown")
	assert.Error(t, err)
}

func TestPageOfAlertGroupSummary(t *testing.T) {
	nextCursor := "next-page-cursor"
	total := 10

	page := oncall.Page[oncall.AlertGroupSummary]{
		Items: []oncall.AlertGroupSummary{
			{
				ID:    "ag-1",
				Title: "Alert 1",
				State: oncall.AlertGroupStateFiring,
			},
		},
		NextCursor:    &nextCursor,
		TotalEstimate: &total,
	}

	assert.Len(t, page.Items, 1)
	assert.Equal(t, "ag-1", page.Items[0].ID)
	assert.NotNil(t, page.NextCursor)
	assert.Equal(t, "next-page-cursor", *page.NextCursor)
	assert.NotNil(t, page.TotalEstimate)
	assert.Equal(t, 10, *page.TotalEstimate)
}
