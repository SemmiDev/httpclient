// Package middleware provides composable http.RoundTripper middleware for the HTTP client.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// RoundTripperFunc is an adapter to allow ordinary functions to implement http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Chain builds a chained RoundTripper from a base transport and a list of wrappers.
// Middleware is applied in the order given (first listed = outermost = called first).
func Chain(base http.RoundTripper, middlewares ...func(http.RoundTripper) http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	for i := len(middlewares) - 1; i >= 0; i-- {
		base = middlewares[i](base)
	}
	return base
}

// --- Request ID ---

type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDFromContext retrieves the request ID injected by RequestIDMiddleware.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// RequestID injects an X-Request-ID header (and context value) if not already present.
// Relies on an external ID generator (e.g. github.com/google/uuid).
func RequestID(generateID func() string) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			id := req.Header.Get("X-Request-ID")
			if id == "" {
				id = generateID()
				req = req.Clone(req.Context())
				req.Header.Set("X-Request-ID", id)
				*req = *req.WithContext(context.WithValue(req.Context(), requestIDKey, id))
			}
			return next.RoundTrip(req)
		})
	}
}

// --- Rate Limiter ---

// RateLimit wraps a transport with a token-bucket rate limiter from golang.org/x/time/rate.
// Each request waits for a token before proceeding.
func RateLimit(limiter *rate.Limiter) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if err := limiter.Wait(req.Context()); err != nil {
				return nil, fmt.Errorf("middleware: rate limiter: %w", err)
			}
			return next.RoundTrip(req)
		})
	}
}

// --- Logging ---

// Logging logs each request and response at Info level using slog.
func Logging(logger *slog.Logger) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			start := time.Now()
			resp, err := next.RoundTrip(req)
			elapsed := time.Since(start)

			attrs := []any{
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Duration("elapsed", elapsed),
			}
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
				logger.ErrorContext(req.Context(), "http request failed", attrs...)
			} else {
				attrs = append(attrs, slog.Int("status", resp.StatusCode))
				logger.InfoContext(req.Context(), "http request", attrs...)
			}
			return resp, err
		})
	}
}

// --- Timeout ---

// Timeout overrides the per-request deadline on the context.
// This is useful when you need a different timeout for specific routes.
func Timeout(d time.Duration) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			ctx, cancel := context.WithTimeout(req.Context(), d)
			defer cancel()
			return next.RoundTrip(req.WithContext(ctx))
		})
	}
}

// --- User Agent ---

// UserAgent injects a fixed User-Agent header on every request.
func UserAgent(ua string) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.Header.Set("User-Agent", ua)
			return next.RoundTrip(req)
		})
	}
}

// --- Header Injection ---

// InjectHeaders injects a fixed set of headers into every request.
func InjectHeaders(headers map[string]string) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			return next.RoundTrip(req)
		})
	}
}

// --- Circuit Breaker adapter ---

// CircuitBreakerController is the interface expected by CircuitBreaker middleware.
type CircuitBreakerController interface {
	Allow() error
	Record(resp *http.Response, err error)
}

// CircuitBreaker wraps a transport with circuit breaker logic.
func CircuitBreaker(cb CircuitBreakerController) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if err := cb.Allow(); err != nil {
				return nil, fmt.Errorf("middleware: %w", err)
			}
			resp, err := next.RoundTrip(req)
			cb.Record(resp, err)
			return resp, err
		})
	}
}
