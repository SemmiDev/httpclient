// Package client provides a production-ready HTTP client built on top of resty
// with support for retries, circuit breaking, rate limiting, structured logging,
// request/response hooks, and ergonomic response parsing.
//
// # Quick Start
//
//	c := client.New(
//	    client.WithBaseURL("https://api.example.com"),
//	    client.WithBearerToken("my-token"),
//	    client.WithTimeout(10*time.Second),
//	    client.WithRetry(client.RetryConfig{MaxAttempts: 3}),
//	)
//
//	var result MyStruct
//	resp, err := c.Get(ctx, "/users/1", client.Bind(&result))
package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client is a thread-safe HTTP client wrapping resty.Client.
// All methods return a *Response which can be used for error checking and decoding.
type Client struct {
	r      *resty.Client
	cfg    *config
	logger *slog.Logger
}

// New creates a new Client applying all given Option functions.
func New(opts ...Option) *Client {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	return newFromConfig(cfg)
}

func newFromConfig(cfg *config) *Client {
	r := resty.New()

	// Base URL
	if cfg.baseURL != "" {
		r.SetBaseURL(cfg.baseURL)
	}

	// Timeout
	r.SetTimeout(cfg.timeout)

	// Default headers
	r.SetHeaders(cfg.headers)

	// Default query params
	r.SetQueryParams(cfg.queryParams)

	// Auth
	if cfg.authToken != "" {
		r.SetAuthToken(cfg.authToken)
	}
	if cfg.authUser != "" {
		r.SetBasicAuth(cfg.authUser, cfg.authPass)
	}

	// TLS
	tlsCfg := cfg.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	// Transport
	transport := cfg.transport
	if transport == nil {
		transport = &http.Transport{
			TLSClientConfig:       tlsCfg,
			Proxy:                 cfg.proxyFunc(),
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: cfg.timeout,
		}
	}
	r.SetTransport(transport)

	// Redirect
	if !cfg.followRedirect {
		r.SetRedirectPolicy(resty.NoRedirectPolicy())
	} else {
		r.SetRedirectPolicy(resty.FlexibleRedirectPolicy(cfg.maxRedirects))
	}

	// Retry
	if cfg.retryConfig != nil && cfg.retryConfig.MaxAttempts > 0 {
		rc := cfg.retryConfig
		r.SetRetryCount(rc.MaxAttempts).
			SetRetryWaitTime(rc.WaitTime).
			SetRetryMaxWaitTime(rc.MaxWaitTime)

		// Default retry on 5xx and network errors
		r.AddRetryCondition(func(resp *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode() >= 500
		})

		// User-defined retry conditions
		for _, cond := range rc.RetryConditions {
			cond := cond // capture loop var
			r.AddRetryCondition(func(resp *resty.Response, err error) bool {
				var rawResp *http.Response
				if resp != nil {
					rawResp = resp.RawResponse
				}
				return cond(rawResp, err)
			})
		}
	}

	// Debug
	r.SetDebug(cfg.debug)

	return &Client{
		r:      r,
		cfg:    cfg,
		logger: cfg.logger,
	}
}

// Resty returns the underlying *resty.Client for advanced use cases.
func (c *Client) Resty() *resty.Client { return c.r }

// Clone returns a new Client that inherits all settings of the current client.
// Useful for creating per-request scoped clients (e.g., with different auth tokens).
func (c *Client) Clone(opts ...Option) *Client {
	// Copy existing config
	cfgCopy := *c.cfg
	cfgCopy.headers = cloneMap(c.cfg.headers)
	cfgCopy.queryParams = cloneMap(c.cfg.queryParams)
	for _, o := range opts {
		o(&cfgCopy)
	}
	return newFromConfig(&cfgCopy)
}

// SetBearerToken updates the bearer token on the existing client (thread-safe via resty).
func (c *Client) SetBearerToken(token string) {
	c.r.SetAuthToken(token)
}

// --- HTTP Methods ---

// Get performs an HTTP GET request.
func (c *Client) Get(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodGet, path, opts...)
}

// Post performs an HTTP POST request.
func (c *Client) Post(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodPost, path, opts...)
}

// Put performs an HTTP PUT request.
func (c *Client) Put(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodPut, path, opts...)
}

// Patch performs an HTTP PATCH request.
func (c *Client) Patch(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodPatch, path, opts...)
}

// Delete performs an HTTP DELETE request.
func (c *Client) Delete(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodDelete, path, opts...)
}

// Head performs an HTTP HEAD request.
func (c *Client) Head(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodHead, path, opts...)
}

// Options performs an HTTP OPTIONS request.
func (c *Client) Options(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.do(ctx, http.MethodOptions, path, opts...)
}

// do is the central dispatcher for all HTTP methods.
func (c *Client) do(ctx context.Context, method, path string, opts ...RequestOption) (*Response, error) {
	req := c.r.R().SetContext(ctx)

	// Apply request options
	ro := &requestOptions{}
	for _, o := range opts {
		o(ro)
	}
	applyRequestOptions(req, ro)

	start := time.Now()
	var (
		restyResp *resty.Response
		err       error
	)

	switch method {
	case http.MethodGet:
		restyResp, err = req.Get(path)
	case http.MethodPost:
		restyResp, err = req.Post(path)
	case http.MethodPut:
		restyResp, err = req.Put(path)
	case http.MethodPatch:
		restyResp, err = req.Patch(path)
	case http.MethodDelete:
		restyResp, err = req.Delete(path)
	case http.MethodHead:
		restyResp, err = req.Head(path)
	case http.MethodOptions:
		restyResp, err = req.Options(path)
	default:
		return nil, fmt.Errorf("httpclient: unsupported method %q", method)
	}

	elapsed := time.Since(start)

	if err != nil {
		c.logger.ErrorContext(ctx, "httpclient: request failed",
			slog.String("method", method),
			slog.String("path", path),
			slog.Duration("elapsed", elapsed),
			slog.String("error", err.Error()),
		)
		return nil, &RequestError{Method: method, URL: path, Err: err}
	}

	resp := newResponse(restyResp)

	c.logger.InfoContext(ctx, "httpclient: request completed",
		slog.String("method", method),
		slog.String("path", path),
		slog.Int("status", resp.StatusCode()),
		slog.Duration("elapsed", elapsed),
	)

	// Auto-decode into bound target if provided
	if ro.bindTarget != nil {
		if err := resp.Decode(ro.bindTarget); err != nil {
			return resp, err
		}
	}

	// Check for error status codes
	if ro.expectSuccess && !resp.IsSuccess() {
		return resp, &HTTPError{
			StatusCode: resp.StatusCode(),
			Status:     resp.Status(),
			Body:       resp.String(),
		}
	}

	return resp, nil
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
