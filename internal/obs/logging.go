package obs

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

const secretMask = "***REDACTED***"

var secretKeys = map[string]bool{
	"token":         true,
	"apikey":        true,
	"api_key":       true,
	"authorization": true,
	"password":      true,
	"secret":        true,
	"credentials":   true,
}

func NewLogger(level string) *slog.Logger {
	handler := &RedactingHandler{
		wrapped: slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: parseLevel(level),
		}),
	}

	return slog.New(handler)
}

type RedactingHandler struct {
	wrapped slog.Handler
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.wrapped.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	redacted := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		value := redactAttr(a)
		redacted.AddAttrs(slog.Attr{Key: a.Key, Value: value})
		return true
	})
	return h.wrapped.Handle(ctx, redacted)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = slog.Attr{Key: a.Key, Value: redactAttr(a)}
	}
	return &RedactingHandler{
		wrapped: h.wrapped.WithAttrs(redacted),
	}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{wrapped: h.wrapped.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Value {
	if isSecretKey(a.Key) {
		switch a.Value.Kind() {
		case slog.KindString:
			if a.Value.String() != "" {
				return slog.StringValue(secretMask)
			}
		case slog.KindAny:
			return slog.StringValue(secretMask)
		}
	}
	return a.Value
}

func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	if secretKeys[lower] {
		return true
	}
	for sk := range secretKeys {
		if strings.Contains(lower, sk) {
			return true
		}
	}
	return false
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
