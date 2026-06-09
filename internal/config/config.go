package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GrafanaURL          string
	ServiceAccountToken string
	OnCallAPIURL        string
	OnCallAPIKey        string
	OnCallReadOnly      bool
	// PluginPreference is the raw string value of
	// GRAFANA_ONCALL_PLUGIN_PREFERENCE / --plugin-preference. The
	// empty string means "no preference; default to legacy
	// grafana-oncall-app when both are installed". Validation of the
	// accepted values (oncall-app | irm) is deferred to
	// oncall.ParsePluginPreference at client-construction time so
	// the config package does not depend on the oncall package.
	PluginPreference          string
	HTTPTimeout               time.Duration
	MaxRetries                int
	LogLevel                  string
	Debug                     bool
	Metrics                   bool
	MetricsAddress            string
	SlowRequestThreshold      time.Duration
	SlowRequestLogLevel       string
	SessionIdleTimeoutMinutes int
	Transport                 string
	Address                   string
	BasePath                  string
	EndpointPath              string
}

func Load() (*Config, error) {
	cfg := &Config{
		GrafanaURL:                os.Getenv("GRAFANA_URL"),
		OnCallAPIURL:              os.Getenv("GRAFANA_ONCALL_API_URL"),
		OnCallAPIKey:              os.Getenv("GRAFANA_ONCALL_API_KEY"),
		OnCallReadOnly:            parseBoolEnv("GRAFANA_ONCALL_READ_ONLY", false),
		PluginPreference:          os.Getenv("GRAFANA_ONCALL_PLUGIN_PREFERENCE"),
		HTTPTimeout:               parseDurationEnv("GRAFANA_ONCALL_HTTP_TIMEOUT", 10*time.Second),
		MaxRetries:                parseIntEnv("GRAFANA_ONCALL_MAX_RETRIES", 3),
		LogLevel:                  "info",
		SlowRequestLogLevel:       "warn",
		SessionIdleTimeoutMinutes: 30,
		Transport:                 "stdio",
		Address:                   "localhost:8000",
		EndpointPath:              "/",
	}

	token := os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN")
	if token == "" {
		token = os.Getenv("GRAFANA_API_KEY")
	}
	cfg.ServiceAccountToken = token

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.UsesDirectOnCallAPI() {
		if c.OnCallAPIURL == "" || c.OnCallAPIKey == "" {
			return fmt.Errorf("GRAFANA_ONCALL_API_URL and GRAFANA_ONCALL_API_KEY must be set together")
		}
		if err := validateURL("GRAFANA_ONCALL_API_URL", c.OnCallAPIURL); err != nil {
			return err
		}
		return nil
	}

	if c.GrafanaURL == "" {
		return fmt.Errorf("GRAFANA_URL is required unless GRAFANA_ONCALL_API_URL and GRAFANA_ONCALL_API_KEY are set")
	}

	if err := validateURL("GRAFANA_URL", c.GrafanaURL); err != nil {
		return err
	}

	if c.ServiceAccountToken == "" {
		return fmt.Errorf("GRAFANA_SERVICE_ACCOUNT_TOKEN or GRAFANA_API_KEY is required")
	}

	return nil
}

// UsesDirectOnCallAPI reports whether the operator configured the
// direct OnCall API mode via GRAFANA_ONCALL_API_URL/API_KEY.
func (c *Config) UsesDirectOnCallAPI() bool {
	return c.OnCallAPIURL != "" || c.OnCallAPIKey != ""
}

func validateURL(name, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", name, rawURL, err)
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return fmt.Errorf("%s must use HTTPS for remote hosts, got %q", name, rawURL)
	}
	return nil
}

func (c *Config) RedactedString() string {
	tokenFingerprint := "none"
	if c.ServiceAccountToken != "" {
		if len(c.ServiceAccountToken) > 4 {
			tokenFingerprint = c.ServiceAccountToken[:4] + "..."
		} else {
			tokenFingerprint = "****"
		}
	}

	directMode := c.UsesDirectOnCallAPI()
	return fmt.Sprintf(
		"Config{URL=%s onCallAPIURL=%s directOnCallAPI=%v token=%s readOnly=%v pluginPreference=%q timeout=%s maxRetries=%d transport=%s}",
		c.GrafanaURL, c.OnCallAPIURL, directMode, tokenFingerprint, c.OnCallReadOnly, c.PluginPreference, c.HTTPTimeout, c.MaxRetries, c.Transport,
	)
}

func (c *Config) String() string {
	return c.RedactedString()
}

func parseBoolEnv(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(strings.ToLower(v))
	if err != nil {
		return defaultVal
	}
	return b
}

func parseIntEnv(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}

func parseDurationEnv(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}
