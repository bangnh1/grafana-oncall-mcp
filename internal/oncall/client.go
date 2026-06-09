package oncall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	aapi "github.com/grafana/amixr-api-go-client"
)

type Client struct {
	httpClient   *http.Client
	onCallClient *aapi.Client
	schedules    *aapi.ScheduleService
	shifts       *aapi.OnCallShiftService
	users        *aapi.UserService
	teams        *aapi.TeamService
	alertGroups  *aapi.AlertGroupService
	plugin       Plugin
}

// ClientOptions carries the optional knobs for NewClientWithOptions.
// All fields are optional; the zero value yields the legacy
// single-plugin (IRM) behaviour with the default timeout.
type ClientOptions struct {
	// HTTPTimeout is the per-request outbound HTTP timeout. Defaults
	// to 10 seconds when zero.
	HTTPTimeout time.Duration
	// PluginPreference is the operator-supplied plugin selection
	// (parsed from GRAFANA_ONCALL_PLUGIN_PREFERENCE / --plugin-preference).
	// The empty PluginPrefUnset is valid and means "default
	// behaviour": prefer the legacy grafana-oncall-app when both
	// are installed.
	PluginPreference PluginPreference
	// Logger receives the startup INFO line that names the
	// selected plugin and the optional WARN line that flags the
	// legacy-preferred-over-IRM default. A nil logger is treated
	// as a no-op.
	Logger *slog.Logger
	// DirectAPIURL is the OnCall application endpoint used with an
	// OnCall API key. When set with DirectAPIKey, plugin auto-detection
	// is skipped and requests go directly to OnCall instead of through
	// Grafana's plugin resource proxy.
	DirectAPIURL string
	// DirectAPIKey is the user-scoped OnCall API key for DirectAPIURL.
	DirectAPIKey string
}

// NewClient preserves the original 001 signature for backward
// compatibility with the existing tests; it constructs the OnCall
// client with the default timeout and the unset plugin preference
// (which, in turn, defers to the dual-plugin selector's default
// behaviour: prefer grafana-oncall-app when both are installed).
func NewClient(grafanaURL, serviceAccountToken string, httpTimeout time.Duration) (*Client, error) {
	return NewClientWithOptions(grafanaURL, serviceAccountToken, ClientOptions{
		HTTPTimeout:      httpTimeout,
		PluginPreference: PluginPrefUnset,
		Logger:           nil,
	})
}

// NewClientWithOptions is the 002 constructor that wires the
// operator's plugin preference through to the dual-plugin selector
// and surfaces the selected plugin in the returned Client. Callers
// that only have the 3-argument form (legacy tests) can keep using
// NewClient; production wiring should switch to this form to
// honor GRAFANA_ONCALL_PLUGIN_PREFERENCE / --plugin-preference.
func NewClientWithOptions(grafanaURL, serviceAccountToken string, opts ClientOptions) (*Client, error) {
	if opts.HTTPTimeout == 0 {
		opts.HTTPTimeout = 10 * time.Second
	}

	if opts.DirectAPIURL != "" || opts.DirectAPIKey != "" {
		if opts.DirectAPIURL == "" || opts.DirectAPIKey == "" {
			return nil, fmt.Errorf("direct OnCall API requires both URL and API key")
		}

		onCallClient, err := aapi.New(opts.DirectAPIURL, opts.DirectAPIKey)
		if err != nil {
			return nil, fmt.Errorf("create direct oncall client: %w", err)
		}
		if opts.Logger != nil {
			opts.Logger.Info("oncall direct api selected", "oncall_api_url", opts.DirectAPIURL)
		}
		return newClientFromAAPI(onCallClient, PluginDirectAPI, opts.HTTPTimeout), nil
	}

	plugin, onCallURL, perr := SelectAndResolve(context.Background(), grafanaURL, serviceAccountToken, opts.PluginPreference, opts.Logger)
	if perr != nil {
		return nil, fmt.Errorf("plugin discovery failed: %w", perr)
	}

	onCallClient, err := aapi.NewWithGrafanaURL(onCallURL, "Bearer "+serviceAccountToken, grafanaURL)
	if err != nil {
		return nil, fmt.Errorf("create oncall client: %w", err)
	}

	return newClientFromAAPI(onCallClient, plugin, opts.HTTPTimeout), nil
}

func newClientFromAAPI(onCallClient *aapi.Client, plugin Plugin, timeout time.Duration) *Client {
	httpClient := &http.Client{Timeout: timeout}
	return &Client{
		httpClient:   httpClient,
		onCallClient: onCallClient,
		schedules:    onCallClient.Schedules,
		shifts:       onCallClient.OnCallShifts,
		users:        onCallClient.Users,
		teams:        onCallClient.Teams,
		alertGroups:  onCallClient.AlertGroups,
		plugin:       plugin,
	}
}

// Plugin returns the plugin selected at startup. The value is
// immutable for the lifetime of the Client (FR-009).
func (c *Client) Plugin() Plugin { return c.plugin }

func (c *Client) Schedules() *aapi.ScheduleService     { return c.schedules }
func (c *Client) Shifts() *aapi.OnCallShiftService     { return c.shifts }
func (c *Client) Users() *aapi.UserService             { return c.users }
func (c *Client) Teams() *aapi.TeamService             { return c.teams }
func (c *Client) AlertGroups() *aapi.AlertGroupService { return c.alertGroups }

func (c *Client) get(ctx context.Context, path string, params any) (*http.Response, error) {
	req, err := c.onCallClient.NewRequest(http.MethodGet, path, params)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Request = req.Request.WithContext(ctx)
	return c.httpClient.Do(req.Request)
}

func (c *Client) GetCurrentOnCallUsers(ctx context.Context, scheduleID string) ([]UserSummary, *time.Time, *http.Response, error) {
	resp, err := c.get(ctx, "on_call_shifts/current_users/", currentOnCallUsersOptions{ScheduleID: scheduleID})
	if err != nil {
		return nil, nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusNotFound {
			users, fallbackResp, fallbackErr := c.getCurrentOnCallUsersFromSchedule(scheduleID)
			if fallbackErr == nil {
				return users, nil, fallbackResp, nil
			}
		}
		return nil, nil, resp, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Results []struct {
			User       oncallUser `json:"user"`
			ShiftEndAt *time.Time `json:"shift_end_at"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, nil, resp, fmt.Errorf("decode current users: %w", err)
	}

	users := make([]UserSummary, 0, len(raw.Results))
	var shiftEnd *time.Time
	for _, r := range raw.Results {
		if r.User.ID != "" {
			users = append(users, UserSummary{ID: r.User.ID, Username: r.User.Username})
		}
		if r.ShiftEndAt != nil {
			shiftEnd = r.ShiftEndAt
		}
	}
	return users, shiftEnd, resp, nil
}

func (c *Client) getCurrentOnCallUsersFromSchedule(scheduleID string) ([]UserSummary, *http.Response, error) {
	schedule, resp, err := c.schedules.GetSchedule(scheduleID, &aapi.GetScheduleOptions{})
	if err != nil {
		return nil, resp, err
	}

	users := make([]UserSummary, 0, len(schedule.OnCallNow))
	for _, userID := range schedule.OnCallNow {
		if userID == "" {
			continue
		}
		users = append(users, UserSummary{ID: userID, Username: userID})
	}
	return users, resp, nil
}

type currentOnCallUsersOptions struct {
	ScheduleID string `url:"schedule_id"`
}

type oncallUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (c *Client) AcknowledgeAlertGroup(ctx context.Context, id string) (*http.Response, error) {
	return c.post(ctx, fmt.Sprintf("/alert_groups/%s/acknowledge/", id), nil)
}

func (c *Client) ResolveAlertGroup(ctx context.Context, id string) (*http.Response, error) {
	return c.post(ctx, fmt.Sprintf("/alert_groups/%s/resolve/", id), nil)
}

func (c *Client) SilenceAlertGroup(ctx context.Context, id string, delaySeconds int) (*http.Response, error) {
	body := map[string]int{"delay": delaySeconds}
	return c.post(ctx, fmt.Sprintf("/alert_groups/%s/silence/", id), body)
}

func (c *Client) UnresolveAlertGroup(ctx context.Context, id string) (*http.Response, error) {
	return c.post(ctx, fmt.Sprintf("/alert_groups/%s/unresolve/", id), nil)
}

func (c *Client) post(ctx context.Context, path string, body any) (*http.Response, error) {
	req, err := c.onCallClient.NewRequest(http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Request = req.Request.WithContext(ctx)
	return c.httpClient.Do(req.Request)
}
