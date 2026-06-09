package oncall

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Timeout     time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   10 * time.Second,
		Timeout:    10 * time.Second,
	}
}

func DoWithRetry(ctx context.Context, fn func(ctx context.Context) (*http.Response, error), cfg RetryConfig) (*http.Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoff(attempt, cfg.BaseDelay, cfg.MaxDelay)
			select {
			case <-reqCtx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", reqCtx.Err())
			case <-time.After(delay):
			}
		}

		resp, err := fn(reqCtx)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter == "" {
				retryAfter = resp.Header.Get("X-RateLimit-Reset")
			}
			lastErr = &RateLimitError{
				Message:    "upstream rate limited",
				RetryAfter: retryAfter,
			}
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("upstream error HTTP %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("exhausted %d retries: %w", cfg.MaxRetries, lastErr)
}

func backoff(attempt int, base, max time.Duration) time.Duration {
	delay := float64(base) * math.Pow(2, float64(attempt-1))
	jitter := float64(base) * rand.Float64()
	d := time.Duration(delay + jitter)
	if d > max {
		d = max
	}
	return d
}

func isRetryableError(err error) bool {
	if isTemporary(err) {
		return true
	}
	return false
}

func isTemporary(err error) bool {
	if te, ok := err.(interface{ Temporary() bool }); ok {
		return te.Temporary()
	}
	return false
}

type RateLimitError struct {
	Message    string
	RetryAfter string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter != "" {
		return fmt.Sprintf("%s (retry after %s)", e.Message, e.RetryAfter)
	}
	return e.Message
}

func ParseRetryAfter(value string) (time.Duration, bool) {
	seconds, err := strconv.Atoi(value)
	if err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	t, err := time.Parse(time.RFC1123, value)
	if err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, true
		}
		return d, true
	}
	return 0, false
}
