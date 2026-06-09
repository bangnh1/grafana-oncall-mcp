package oncall

import "time"

type Cursor string

type ScheduleType string

const (
	ScheduleTypeWeb      ScheduleType = "web"
	ScheduleTypeCalendar ScheduleType = "calendar"
	ScheduleTypeIcal     ScheduleType = "ical"
)

type ShiftType string

const (
	ShiftTypeSingleEvent    ShiftType = "single_event"
	ShiftTypeRecurrentEvent ShiftType = "recurrent_event"
	ShiftTypeRollingUsers   ShiftType = "rolling_users"
)

type ShiftFrequency string

const (
	ShiftFrequencyDaily   ShiftFrequency = "daily"
	ShiftFrequencyWeekly  ShiftFrequency = "weekly"
	ShiftFrequencyMonthly ShiftFrequency = "monthly"
)

type UserRole string

const (
	UserRoleAdmin  UserRole = "admin"
	UserRoleEditor UserRole = "editor"
	UserRoleViewer UserRole = "viewer"
)

type AlertGroupState string

const (
	AlertGroupStateNew          AlertGroupState = "new"
	AlertGroupStateFiring       AlertGroupState = AlertGroupStateNew
	AlertGroupStateAcknowledged AlertGroupState = "acknowledged"
	AlertGroupStateResolved     AlertGroupState = "resolved"
	AlertGroupStateSilenced     AlertGroupState = "silenced"
)

type UserSummary struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
}

type User struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       *string  `json:"email"`
	DisplayName *string  `json:"display_name"`
	Role        *string  `json:"role"`
	Timezone    *string  `json:"timezone"`
	TeamIDs     []string `json:"team_ids"`
}

type Team struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     *string `json:"email"`
	AvatarURL *string `json:"avatar_url"`
}

type Schedule struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	TeamID              *string      `json:"team_id"`
	Type                ScheduleType `json:"type"`
	Timezone            string       `json:"timezone"`
	TimezoneWasInferred bool         `json:"timezone_was_inferred"`
	ShiftIDs            []string     `json:"shift_ids"`
}

type Shift struct {
	ID                         string        `json:"id"`
	ScheduleID                 string        `json:"schedule_id"`
	Name                       *string       `json:"name,omitempty"`
	Type                       ShiftType     `json:"type"`
	StartAt                    time.Time     `json:"start_at"`
	EndAt                      *time.Time    `json:"end_at,omitempty"`
	Duration                   string        `json:"duration"`
	Frequency                  *string       `json:"frequency,omitempty"`
	Interval                   *int          `json:"interval,omitempty"`
	WeekStart                  *string       `json:"week_start,omitempty"`
	ByDay                      []string      `json:"by_day,omitempty"`
	ByMonth                    []int         `json:"by_month,omitempty"`
	ByMonthday                 []int         `json:"by_monthday,omitempty"`
	RollingUsers               [][]string    `json:"rolling_users,omitempty"`
	Users                      []UserSummary `json:"users,omitempty"`
	TimeZone                   *string       `json:"time_zone,omitempty"`
	Level                      int           `json:"level,omitempty"`
	StartRotationFromUserIndex *int          `json:"start_rotation_from_user_index,omitempty"`
	RotationStart              *time.Time    `json:"rotation_start,omitempty"`
}

type Integration struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type AlertGroupSummary struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	State           AlertGroupState `json:"state"`
	Severity        *string         `json:"severity"`
	IntegrationID   string          `json:"integration_id"`
	IntegrationName string          `json:"integration_name"`
	RouteID         *string         `json:"route_id"`
	TeamID          *string         `json:"team_id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ResolvedAt      *time.Time      `json:"resolved_at"`
	AcknowledgedAt  *time.Time      `json:"acknowledged_at"`
	AcknowledgedBy  *UserSummary    `json:"acknowledged_by"`
	SilencedUntil   *time.Time      `json:"silenced_until"`
	AlertsCount     int             `json:"alerts_count"`
}

type AlertGroup struct {
	AlertGroupSummary
	Labels    StringMap `json:"labels"`
	Permalink string    `json:"permalink"`
}

type StringMap map[string]string

type Page[T any] struct {
	Items         []T     `json:"items"`
	NextCursor    *string `json:"next_cursor"`
	TotalEstimate *int    `json:"total_estimate"`
}

type WriteResult struct {
	AlertGroupID      string          `json:"alert_group_id"`
	NewState          AlertGroupState `json:"new_state"`
	WasAlreadyInState bool            `json:"was_already_in_state"`
	ActingUser        UserSummary     `json:"acting_user"`
	PerformedAt       time.Time       `json:"performed_at"`
}

type SilenceResult struct {
	WriteResult
	SilencedUntil time.Time `json:"silenced_until"`
}

type CurrentOnCallResult struct {
	ScheduleID string        `json:"schedule_id"`
	AsOf       time.Time     `json:"as_of"`
	Users      []UserSummary `json:"users"`
	ShiftEndAt *time.Time    `json:"shift_end_at"`
}
