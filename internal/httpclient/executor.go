package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/rate"
)

// Backoff returns the retry sleep duration for the given attempt number.
func Backoff(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 100 * time.Millisecond
	case 1:
		return 250 * time.Millisecond
	default:
		return 500 * time.Millisecond
	}
}

// Executor handles rate-limited, retrying HTTP execution with JSON decoding.
type Executor struct {
	logger       *zap.Logger
	rateMgr      *rate.Manager
	http         *http.Client
	retryMax     int
	venueTag     string
	errorHandler func(status int, body []byte) error
}

// New creates an Executor. errorHandler is called on 4xx failure responses to produce a
// venue-specific error. If nil, a default error is returned.
func New(
	logger *zap.Logger,
	rateMgr *rate.Manager,
	httpClient *http.Client,
	retryMax int,
	venueTag string,
	errorHandler func(status int, body []byte) error,
) *Executor {
	return &Executor{
		logger:       logger,
		rateMgr:      rateMgr,
		http:         httpClient,
		retryMax:     retryMax,
		venueTag:     venueTag,
		errorHandler: errorHandler,
	}
}

// DoJSON executes req with rate limiting and retries, then JSON-decodes the response into out.
// rateLimitKey scopes the rate limiter per client/venue.
func (e *Executor) DoJSON(ctx context.Context, req *http.Request, rateLimitKey string, out any) error {
	if e.rateMgr != nil {
		if err := e.rateMgr.Wait(ctx, rateLimitKey); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= e.retryMax; attempt++ {
		start := time.Now()
		resp, err := e.http.Do(req)
		if err != nil {
			lastErr = err
			e.logger.Warn(e.venueTag+".http_failed",
				zap.String("url", req.URL.String()),
				zap.Error(err),
				zap.Int("attempt", attempt))
			time.Sleep(Backoff(attempt))
			continue
		}
		defer func() { _ = resp.Body.Close() }() //nolint:gocritic

		body, _ := io.ReadAll(resp.Body)
		elapsed := time.Since(start)

		if resp.StatusCode >= 500 {
			e.logger.Warn(e.venueTag+".server_error",
				zap.Int("status", resp.StatusCode),
				zap.String("url", req.URL.String()),
				zap.Duration("latency", elapsed))
			lastErr = fmt.Errorf("%s server error: %d", e.venueTag, resp.StatusCode)
			time.Sleep(Backoff(attempt))
			continue
		}

		if resp.StatusCode >= 400 {
			if e.errorHandler != nil {
				return e.errorHandler(resp.StatusCode, body)
			}
			return fmt.Errorf("%s returned %d", e.venueTag, resp.StatusCode)
		}

		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				e.logger.Warn(e.venueTag+".decode_failed",
					zap.Error(err),
					zap.String("url", req.URL.String()),
					zap.String("body", string(body)))
				return fmt.Errorf("decode failed: %w", err)
			}
		}

		e.logger.Debug(e.venueTag+".http_success",
			zap.String("url", req.URL.String()),
			zap.Int("status", resp.StatusCode),
			zap.Duration("elapsed", elapsed))

		return nil
	}

	return fmt.Errorf("%s request failed after %d attempts: %w", e.venueTag, e.retryMax, lastErr)
}
