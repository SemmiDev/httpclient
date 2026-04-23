// Package mock provides testing utilities for the httpclient package.
// Use MockTransport to intercept HTTP requests without hitting a real server.
package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
)

// Handler is a function that handles a mocked HTTP call.
type Handler func(req *http.Request) (*http.Response, error)

// Route maps a method+path pair to a Handler.
type Route struct {
	Method  string
	Path    string
	Handler Handler
}

// Transport is a mock http.RoundTripper that intercepts calls and routes them to Handlers.
// It is safe for concurrent use and records all requests for assertion.
type Transport struct {
	mu       sync.Mutex
	routes   []Route
	fallback Handler
	calls    []*http.Request
	t        *testing.T
}

// NewTransport creates a new MockTransport for use in tests.
func NewTransport(t *testing.T) *Transport {
	t.Helper()
	return &Transport{t: t}
}

// On registers a handler for the given method and path.
// Path matching is exact. Register more specific routes first.
func (m *Transport) On(method, path string, h Handler) *Transport {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = append(m.routes, Route{Method: method, Path: path, Handler: h})
	return m
}

// Fallback sets a handler called when no route matches.
func (m *Transport) Fallback(h Handler) *Transport {
	m.fallback = h
	return m
}

// RoundTrip implements http.RoundTripper.
func (m *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	m.calls = append(m.calls, req.Clone(req.Context()))
	m.mu.Unlock()

	for _, route := range m.routes {
		if route.Method == req.Method && route.Path == req.URL.Path {
			return route.Handler(req)
		}
	}
	if m.fallback != nil {
		return m.fallback(req)
	}
	m.t.Errorf("mock: unhandled request: %s %s", req.Method, req.URL.Path)
	return nil, fmt.Errorf("mock: no handler for %s %s", req.Method, req.URL.Path)
}

// Calls returns all recorded requests (for assertion).
func (m *Transport) Calls() []*http.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*http.Request, len(m.calls))
	copy(out, m.calls)
	return out
}

// CallCount returns the number of requests intercepted.
func (m *Transport) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// AssertCalled asserts that at least one request was made to the given method+path.
func (m *Transport) AssertCalled(method, path string) {
	m.t.Helper()
	for _, r := range m.Calls() {
		if r.Method == method && r.URL.Path == path {
			return
		}
	}
	m.t.Errorf("mock: expected call to %s %s, but it was never made", method, path)
}

// AssertCallCount asserts the total number of intercepted requests.
func (m *Transport) AssertCallCount(n int) {
	m.t.Helper()
	if got := m.CallCount(); got != n {
		m.t.Errorf("mock: expected %d call(s), got %d", n, got)
	}
}

// Reset clears recorded calls and registered routes.
func (m *Transport) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	m.routes = nil
}

// --- Response builder helpers ---

// JSONResponse builds an *http.Response with a JSON body.
func JSONResponse(statusCode int, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader(b)),
	}, nil
}

// TextResponse builds an *http.Response with a plain text body.
func TextResponse(statusCode int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}, nil
}

// EmptyResponse builds an *http.Response with no body.
func EmptyResponse(statusCode int) (*http.Response, error) {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

// ErrorResponse returns a simulated network error (e.g. timeout, DNS failure).
func ErrorResponse(err error) (*http.Response, error) {
	return nil, err
}
