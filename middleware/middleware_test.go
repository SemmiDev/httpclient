package middleware_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/semmidev/httpclient/middleware"
)

// okTransport always returns 200 OK.
var okTransport middleware.RoundTripperFunc = func(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{},
	}, nil
}

// errTransport always returns an error.
var errTransport middleware.RoundTripperFunc = func(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("network error")
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", nil)
	return req
}

// --- Chain ---

func TestChain_AppliesInOrder(t *testing.T) {
	order := []string{}

	a := func(next http.RoundTripper) http.RoundTripper {
		return middleware.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			order = append(order, "a-before")
			resp, err := next.RoundTrip(r)
			order = append(order, "a-after")
			return resp, err
		})
	}
	b := func(next http.RoundTripper) http.RoundTripper {
		return middleware.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			order = append(order, "b-before")
			resp, err := next.RoundTrip(r)
			order = append(order, "b-after")
			return resp, err
		})
	}

	transport := middleware.Chain(okTransport, a, b)
	_, _ = transport.RoundTrip(newRequest(t))

	want := []string{"a-before", "b-before", "b-after", "a-after"}
	for i, got := range order {
		if got != want[i] {
			t.Errorf("order[%d]: want %q, got %q", i, want[i], got)
		}
	}
}

// --- UserAgent ---

func TestUserAgent_SetsHeader(t *testing.T) {
	var gotUA string
	capture := middleware.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		gotUA = r.Header.Get("User-Agent")
		return okTransport.RoundTrip(r)
	})

	transport := middleware.Chain(capture, middleware.UserAgent("TestClient/1.0"))
	_, _ = transport.RoundTrip(newRequest(t))

	if gotUA != "TestClient/1.0" {
		t.Errorf("expected User-Agent=TestClient/1.0, got %q", gotUA)
	}
}

// --- InjectHeaders ---

func TestInjectHeaders(t *testing.T) {
	var gotHeader string
	capture := middleware.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		gotHeader = r.Header.Get("X-Custom")
		return okTransport.RoundTrip(r)
	})

	transport := middleware.Chain(capture, middleware.InjectHeaders(map[string]string{"X-Custom": "value123"}))
	_, _ = transport.RoundTrip(newRequest(t))

	if gotHeader != "value123" {
		t.Errorf("expected X-Custom=value123, got %q", gotHeader)
	}
}

// --- Timeout ---

func TestTimeout_CancelsSlowRequest(t *testing.T) {
	slow := middleware.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-time.After(5 * time.Second):
			return okTransport.RoundTrip(r)
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})

	transport := middleware.Chain(slow, middleware.Timeout(20*time.Millisecond))
	_, err := transport.RoundTrip(newRequest(t))

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// --- RateLimit ---

func TestRateLimit_AllowsRequestsWithinLimit(t *testing.T) {
	limiter := rate.NewLimiter(rate.Every(10*time.Millisecond), 5)
	transport := middleware.Chain(okTransport, middleware.RateLimit(limiter))

	for i := 0; i < 5; i++ {
		_, err := transport.RoundTrip(newRequest(t))
		if err != nil {
			t.Errorf("request %d unexpected error: %v", i, err)
		}
	}
}

func TestRateLimit_BlocksWhenContextCancelled(t *testing.T) {
	// Limiter exhausted immediately
	limiter := rate.NewLimiter(rate.Every(10*time.Second), 0)
	transport := middleware.Chain(okTransport, middleware.RateLimit(limiter))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	_, err := transport.RoundTrip(req)

	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
}

// --- CircuitBreaker ---

type mockCB struct {
	allowErr error
	recorded atomic.Int32
}

func (m *mockCB) Allow() error                     { return m.allowErr }
func (m *mockCB) Record(_ *http.Response, _ error) { m.recorded.Add(1) }

func TestCircuitBreaker_BlocksWhenOpen(t *testing.T) {
	cb := &mockCB{allowErr: errors.New("circuit open")}
	transport := middleware.Chain(okTransport, middleware.CircuitBreaker(cb))

	_, err := transport.RoundTrip(newRequest(t))
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if cb.recorded.Load() != 0 {
		t.Error("record should not be called when circuit is open")
	}
}

func TestCircuitBreaker_RecordsOnSuccess(t *testing.T) {
	cb := &mockCB{allowErr: nil}
	transport := middleware.Chain(okTransport, middleware.CircuitBreaker(cb))

	_, err := transport.RoundTrip(newRequest(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.recorded.Load() != 1 {
		t.Error("expected Record to be called once")
	}
}

func TestCircuitBreaker_RecordsOnTransportError(t *testing.T) {
	cb := &mockCB{allowErr: nil}
	transport := middleware.Chain(errTransport, middleware.CircuitBreaker(cb))

	_, _ = transport.RoundTrip(newRequest(t))

	if cb.recorded.Load() != 1 {
		t.Error("expected Record to be called on error")
	}
}
