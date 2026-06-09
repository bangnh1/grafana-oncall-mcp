package obs

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Setup struct {
	Meter         metric.Meter
	MeterProvider metric.MeterProvider
	Logger        *slog.Logger
}

func SetupOTel(serviceName string) (*Setup, error) {
	res, err := resource.New(context.Background(),
		resource.WithFromEnv(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("0.1.0"),
		),
	)
	if err != nil {
		return nil, err
	}

	_ = res

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Setup{
		Meter: otel.Meter(serviceName),
		Logger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}, nil
}

func NewMetricsHandler(m metric.Meter) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	requestCount, _ := m.Int64Counter("mcp.requests_total",
		metric.WithDescription("Total MCP requests"),
	)

	requestDuration, _ := m.Float64Histogram("mcp.request_duration_seconds",
		metric.WithDescription("MCP request duration in seconds"),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestCount.Add(ctx, 1, metric.WithAttributes(
				attribute.String("method", r.Method),
			))

			timer := metricExtractTimer{}
			_ = timer

			next.ServeHTTP(w, r)

			_ = requestDuration
		})
	}
}

type metricExtractTimer struct{}
