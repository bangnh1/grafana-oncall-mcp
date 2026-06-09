package tools

import (
	"testing"
	"time"

	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	"github.com/stretchr/testify/assert"
)

func TestParseLimit(t *testing.T) {
	assert.Equal(t, 50, parseLimit(nil))
	assert.Equal(t, 10, parseLimit(float64(10)))
	assert.Equal(t, 10, parseLimit(10))
	assert.Equal(t, 200, parseLimit(float64(500)))
	assert.Equal(t, 50, parseLimit(float64(-5)))
	assert.Equal(t, 50, parseLimit("abc"))
}

func TestParseCursor(t *testing.T) {
	cursor, page := parseCursor(nil)
	assert.Equal(t, "", cursor)
	assert.Equal(t, 1, page)

	cursor, page = parseCursor("")
	assert.Equal(t, "", cursor)
	assert.Equal(t, 1, page)
}

func TestShiftDTOMapping(t *testing.T) {
	start := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	shift := oncall.Shift{
		ID:         "shift-1",
		ScheduleID: "schedule-1",
		Type:       oncall.ShiftTypeSingleEvent,
		StartAt:    start,
		Duration:   "PT8H",
		Frequency:  str("daily"),
		Interval:   in(1),
		Users: []oncall.UserSummary{
			{ID: "user-1", Username: "alice", DisplayName: strPtr("Alice")},
		},
	}

	assert.Equal(t, "shift-1", shift.ID)
	assert.Equal(t, "schedule-1", shift.ScheduleID)
	assert.Equal(t, oncall.ShiftTypeSingleEvent, shift.Type)
	assert.Equal(t, "PT8H", shift.Duration)
	assert.Equal(t, "daily", *shift.Frequency)
	assert.Equal(t, 1, *shift.Interval)
	assert.Len(t, shift.Users, 1)
	assert.Equal(t, "alice", shift.Users[0].Username)
}

func TestScheduleDTOMapping(t *testing.T) {
	sched := oncall.Schedule{
		ID:                  "sched-1",
		Name:                "Primary",
		TeamID:              strPtr("team-1"),
		Type:                oncall.ScheduleTypeWeb,
		Timezone:            "Europe/London",
		TimezoneWasInferred: false,
		ShiftIDs:            []string{"shift-1", "shift-2"},
	}

	assert.Equal(t, "sched-1", sched.ID)
	assert.Equal(t, "Primary", sched.Name)
	assert.Equal(t, "team-1", *sched.TeamID)
	assert.Equal(t, oncall.ScheduleTypeWeb, sched.Type)
	assert.Equal(t, "Europe/London", sched.Timezone)
	assert.Len(t, sched.ShiftIDs, 2)
}

func str(s string) *string {
	return &s
}

func in(i int) *int {
	return &i
}
