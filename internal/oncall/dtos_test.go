package oncall

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleJSONRoundTrip(t *testing.T) {
	s := Schedule{
		ID:                  "sched-1",
		Name:                "Database on-call",
		TeamID:              strPtr("team-1"),
		Type:                ScheduleTypeWeb,
		Timezone:            "Europe/London",
		TimezoneWasInferred: false,
		ShiftIDs:            []string{"shift-1", "shift-2"},
	}

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var got Schedule
	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, s.Name, got.Name)
	assert.Equal(t, *s.TeamID, *got.TeamID)
	assert.Equal(t, s.Type, got.Type)
	assert.Equal(t, s.Timezone, got.Timezone)
	assert.Equal(t, s.ShiftIDs, got.ShiftIDs)
}

func TestShiftJSONRoundTrip(t *testing.T) {
	start := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	rotStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := Shift{
		ID:         "shift-1",
		ScheduleID: "sched-1",
		Name:       strPtr("Morning"),
		Type:       ShiftTypeSingleEvent,
		StartAt:    start,
		Duration:   "PT8H",
		Frequency:  strPtr("daily"),
		Interval:   intPtr(1),
		Users: []UserSummary{
			{ID: "user-1", Username: "alice", DisplayName: strPtr("Alice")},
		},
		RotationStart: &rotStart,
	}

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var got Shift
	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, s.StartAt.Unix(), got.StartAt.Unix())
	assert.Equal(t, s.Duration, got.Duration)
	assert.Equal(t, s.Users, got.Users)
}

func TestAlertGroupStateEnum(t *testing.T) {
	states := []AlertGroupState{
		AlertGroupStateFiring,
		AlertGroupStateAcknowledged,
		AlertGroupStateResolved,
		AlertGroupStateSilenced,
	}
	for _, state := range states {
		var s AlertGroupState = state
		assert.NotEmpty(t, string(s))
	}
}

func TestUserSummaryFields(t *testing.T) {
	u := UserSummary{
		ID:          "u1",
		Username:    "bob",
		DisplayName: strPtr("Bob"),
	}
	assert.Equal(t, "u1", u.ID)
	assert.Equal(t, "bob", u.Username)
	assert.Equal(t, "Bob", *u.DisplayName)

	nullDisplay := UserSummary{ID: "u2", Username: "carol"}
	assert.Nil(t, nullDisplay.DisplayName)
}

func TestUserSummaryJSONRoundTrip(t *testing.T) {
	u := UserSummary{ID: "u1", Username: "alice", DisplayName: strPtr("Alice")}
	data, err := json.Marshal(u)
	require.NoError(t, err)
	var got UserSummary
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, u.Username, got.Username)
	assert.Equal(t, "Alice", *got.DisplayName)
}

func TestUserJSONRoundTrip(t *testing.T) {
	u := User{
		ID:          "u1",
		Username:    "alice",
		Email:       strPtr("alice@example.com"),
		DisplayName: strPtr("Alice"),
		Role:        strPtr("admin"),
		Timezone:    strPtr("Europe/London"),
		TeamIDs:     []string{"team-1"},
	}
	data, err := json.Marshal(u)
	require.NoError(t, err)
	var got User
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, u.Username, got.Username)
	assert.Equal(t, "alice@example.com", *got.Email)
	assert.Equal(t, "admin", *got.Role)
	assert.Equal(t, []string{"team-1"}, got.TeamIDs)
}

func TestUserNullFields(t *testing.T) {
	u := User{ID: "u1", Username: "bob"}
	assert.Nil(t, u.Email)
	assert.Nil(t, u.DisplayName)
	assert.Nil(t, u.Role)
	assert.Nil(t, u.Timezone)
	assert.Empty(t, u.TeamIDs)
}

func TestTeamJSONRoundTrip(t *testing.T) {
	tm := Team{
		ID:        "team-1",
		Name:      "Engineering",
		Email:     strPtr("eng@example.com"),
		AvatarURL: strPtr("https://example.com/avatar.png"),
	}
	data, err := json.Marshal(tm)
	require.NoError(t, err)
	var got Team
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, tm.ID, got.ID)
	assert.Equal(t, tm.Name, got.Name)
	assert.Equal(t, "eng@example.com", *got.Email)
	assert.Equal(t, "https://example.com/avatar.png", *got.AvatarURL)
}

func TestTeamNullFields(t *testing.T) {
	tm := Team{ID: "team-1", Name: "Ops"}
	assert.Nil(t, tm.Email)
	assert.Nil(t, tm.AvatarURL)
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
