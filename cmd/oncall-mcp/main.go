// Command grafana-oncall-mcp is a standalone Model Context Protocol
// server that exposes the Grafana OnCall API (via either the
// legacy `grafana-oncall-app` plugin or the rebranded
// `grafana-irm-app` plugin, auto-detected at startup) to AI
// assistants and other MCP clients.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/bangnh1/grafana-oncall-mcp/internal/config"
	"github.com/bangnh1/grafana-oncall-mcp/internal/obs"
	"github.com/bangnh1/grafana-oncall-mcp/internal/oncall"
	"github.com/bangnh1/grafana-oncall-mcp/internal/server"
	"github.com/bangnh1/grafana-oncall-mcp/internal/tools"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	transport            = flag.String("transport", "stdio", "MCP transport: stdio, sse, or streamable-http")
	address              = flag.String("address", "localhost:8000", "Listen address for sse/streamable-http")
	basePath             = flag.String("base-path", "", "SSE base path")
	endpointPath         = flag.String("endpoint-path", "/", "Streamable HTTP endpoint path")
	readOnly             = flag.Bool("read-only", false, "Suppress all write tools")
	logLevel             = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	debug                = flag.Bool("debug", false, "Enable verbose HTTP request/response logging")
	metricsFlag          = flag.Bool("metrics", false, "Enable /metrics endpoint")
	metricsAddress       = flag.String("metrics-address", "", "Separate metrics listener address")
	slowRequestThreshold = flag.Duration("slow-request-threshold", 0, "Log requests exceeding this duration")
	slowRequestLogLevel  = flag.String("slow-request-log-level", "warn", "Log level for slow requests: info or warn")
	sessionIdleTimeout   = flag.Int("session-idle-timeout-minutes", 30, "Session idle timeout for sse/streamable-http")
	pluginPreference     = flag.String("plugin-preference", "", "Force plugin selection: oncall-app, irm, or unset (default: prefer oncall-app when both are installed)")
	printVersion         = flag.Bool("version", false, "Print version and exit")
)

// version is the server version. The default is overridden at
// link time by goreleaser via `-ldflags "-X main.version=vX.Y.Z"`
// (see .goreleaser.yaml). When running from `go run` or a
// hand-built binary without ldflags, this constant is the
// reported version.
var version = "0.1.0"

type startupError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func main() {
	flag.Parse()

	if *printVersion {
		fmt.Println("grafana-oncall-mcp " + version)
		return
	}

	logger := obs.NewLogger(*logLevel)

	cfg, err := config.Load()
	if err != nil {
		writeStartupError(logger, "CONFIG_LOAD_FAILED", err.Error())
		os.Exit(1)
	}

	cfg.Transport = *transport
	cfg.Address = *address
	cfg.BasePath = *basePath
	cfg.EndpointPath = *endpointPath
	cfg.OnCallReadOnly = cfg.OnCallReadOnly || *readOnly
	cfg.LogLevel = *logLevel
	cfg.Debug = *debug
	cfg.Metrics = *metricsFlag
	cfg.MetricsAddress = *metricsAddress
	cfg.SlowRequestThreshold = *slowRequestThreshold
	cfg.SlowRequestLogLevel = *slowRequestLogLevel
	cfg.SessionIdleTimeoutMinutes = *sessionIdleTimeout

	// Flag wins over env var, identical to the --read-only / GRAFANA_ONCALL_READ_ONLY
	// rule on line 64 above. An empty flag value means "use the env var".
	if *pluginPreference != "" {
		cfg.PluginPreference = *pluginPreference
	}

	if err := cfg.Validate(); err != nil {
		writeStartupError(logger, "INVALID_CONFIG", err.Error())
		os.Exit(1)
	}

	logger.Info("starting grafana-oncall-mcp", "config", cfg)

	if cfg.Metrics {
		addr := cfg.MetricsAddress
		if addr == "" {
			addr = ":9090"
		}
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			logger.Info("metrics endpoint listening", "address", addr)
			if err := http.ListenAndServe(addr, mux); err != nil {
				logger.Error("metrics server error", "error", err)
			}
		}()
	}

	slogLevel := slog.LevelInfo
	switch *slowRequestLogLevel {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	}

	srv := server.NewServer(server.ServerOpts{
		Logger:               logger,
		ReadOnly:             cfg.OnCallReadOnly,
		SlowRequestThreshold: cfg.SlowRequestThreshold,
		SlowRequestLogLevel:  slogLevel,
	})

	// Parse the operator-supplied plugin preference only for Grafana
	// plugin mode. Direct OnCall API mode bypasses plugin selection.
	pref := oncall.PluginPrefUnset
	if !cfg.UsesDirectOnCallAPI() {
		var perr *oncall.Error
		pref, perr = oncall.ParsePluginPreference(cfg.PluginPreference)
		if perr != nil {
			writeStartupError(logger, "INVALID_CONFIG", perr.Message)
			if perr.Hint != nil {
				fmt.Fprintf(os.Stderr, "hint: %s\n", *perr.Hint)
			}
			os.Exit(1)
		}
	}

	ocClient, err := oncall.NewClientWithOptions(cfg.GrafanaURL, cfg.ServiceAccountToken, oncall.ClientOptions{
		HTTPTimeout:      cfg.HTTPTimeout,
		PluginPreference: pref,
		Logger:           logger,
		DirectAPIURL:     cfg.OnCallAPIURL,
		DirectAPIKey:     cfg.OnCallAPIKey,
	})
	if err != nil {
		writeStartupError(logger, "STARTUP_HEALTH_CHECK_FAILED", err.Error())
		os.Exit(1)
	}
	logger.Info("client initialized", "plugin", ocClient.Plugin().String())

	tools.AddOnCallTools(srv, ocClient)

	var tp server.Transport
	tp, err = server.ParseTransport(cfg.Transport)
	if err != nil {
		logger.Error("invalid transport", "error", err)
		os.Exit(1)
	}

	logger.Info("serving", "transport", tp)
	if err := server.Serve(srv.MCPServer(), server.TransportOpts{
		Transport:                 tp,
		Address:                   cfg.Address,
		BasePath:                  cfg.BasePath,
		EndpointPath:              cfg.EndpointPath,
		SessionIdleTimeoutMinutes: cfg.SessionIdleTimeoutMinutes,
		Logger:                    logger,
	}); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func writeStartupError(logger *slog.Logger, code, message string) {
	logger.Error("startup failed", "code", code, "message", message)
	if data, err := json.Marshal(startupError{Code: code, Message: message}); err == nil {
		os.Stderr.WriteString(string(data) + "\n")
	}
}
