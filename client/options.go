package client

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Option configures a Client via functional options pattern.
type Option func(*config)

type config struct {
	baseURL        string
	timeout        time.Duration
	headers        map[string]string
	queryParams    map[string]string
	authToken      string
	authUser       string
	authPass       string
	retryConfig    *RetryConfig
	tlsConfig      *tls.Config
	proxyURL       string
	logger         *slog.Logger
	rateLimiter    RateLimiter
	hooks          Hooks
	debug          bool
	followRedirect bool
	maxRedirects   int
	transport      http.RoundTripper
}

// RetryConfig holds retry policy settings.
type RetryConfig struct {
	MaxAttempts     int
	WaitTime        time.Duration
	MaxWaitTime     time.Duration
	RetryConditions []RetryConditionFunc
}

// RetryConditionFunc determines whether a response should be retried.
type RetryConditionFunc func(*http.Response, error) bool

// RateLimiter is satisfied by golang.org/x/time/rate.Limiter or any custom impl.
type RateLimiter interface {
	Wait(ctx interface{ Deadline() (time.Time, bool) }) error
}

// Hooks holds lifecycle callback functions.
type Hooks struct {
	OnBeforeRequest []BeforeRequestHook
	OnAfterResponse []AfterResponseHook
	OnError         []ErrorHook
}

type BeforeRequestHook func(req *http.Request) error
type AfterResponseHook func(resp *http.Response) error
type ErrorHook func(req *http.Request, err error)

func defaultConfig() *config {
	return &config{
		timeout:        30 * time.Second,
		headers:        make(map[string]string),
		queryParams:    make(map[string]string),
		followRedirect: true,
		maxRedirects:   10,
		logger:         slog.Default(),
		retryConfig: &RetryConfig{
			MaxAttempts: 0, // disabled by default
			WaitTime:    100 * time.Millisecond,
			MaxWaitTime: 2 * time.Second,
		},
	}
}

// --- Option constructors ---

// WithBaseURL sets the base URL for all requests.
func WithBaseURL(u string) Option {
	return func(c *config) { c.baseURL = u }
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithHeader adds a default header sent on every request.
func WithHeader(key, value string) Option {
	return func(c *config) { c.headers[key] = value }
}

// WithHeaders adds multiple default headers.
func WithHeaders(headers map[string]string) Option {
	return func(c *config) {
		for k, v := range headers {
			c.headers[k] = v
		}
	}
}

// WithQueryParam adds a default query parameter to every request.
func WithQueryParam(key, value string) Option {
	return func(c *config) { c.queryParams[key] = value }
}

// WithBearerToken sets the Authorization: Bearer <token> header.
func WithBearerToken(token string) Option {
	return func(c *config) { c.authToken = token }
}

// WithBasicAuth sets HTTP Basic Authentication credentials.
func WithBasicAuth(username, password string) Option {
	return func(c *config) {
		c.authUser = username
		c.authPass = password
	}
}

// WithRetry configures the retry policy.
func WithRetry(cfg RetryConfig) Option {
	return func(c *config) { c.retryConfig = &cfg }
}

// WithTLSConfig sets a custom TLS configuration.
func WithTLSConfig(tlsCfg *tls.Config) Option {
	return func(c *config) { c.tlsConfig = tlsCfg }
}

// WithTLSInsecureSkipVerify disables TLS certificate verification.
// WARNING: Only use in development/testing.
func WithTLSInsecureSkipVerify() Option {
	return func(c *config) {
		c.tlsConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
}

// WithProxy sets the proxy URL for all requests.
func WithProxy(proxyURL string) Option {
	return func(c *config) { c.proxyURL = proxyURL }
}

// WithLogger sets a custom slog.Logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithDebug enables debug logging of requests and responses.
func WithDebug(enabled bool) Option {
	return func(c *config) { c.debug = enabled }
}

// WithFollowRedirect controls whether redirects are followed.
func WithFollowRedirect(follow bool, maxRedirects int) Option {
	return func(c *config) {
		c.followRedirect = follow
		c.maxRedirects = maxRedirects
	}
}

// WithTransport sets a custom http.RoundTripper.
func WithTransport(t http.RoundTripper) Option {
	return func(c *config) { c.transport = t }
}

// WithBeforeRequestHook registers a hook called before each request.
func WithBeforeRequestHook(fn BeforeRequestHook) Option {
	return func(c *config) { c.hooks.OnBeforeRequest = append(c.hooks.OnBeforeRequest, fn) }
}

// WithAfterResponseHook registers a hook called after each response.
func WithAfterResponseHook(fn AfterResponseHook) Option {
	return func(c *config) { c.hooks.OnAfterResponse = append(c.hooks.OnAfterResponse, fn) }
}

// WithErrorHook registers a hook called on request errors.
func WithErrorHook(fn ErrorHook) Option {
	return func(c *config) { c.hooks.OnError = append(c.hooks.OnError, fn) }
}

// proxyFunc returns an http.ProxyFromEnvironment-compatible func.
func (c *config) proxyFunc() func(*http.Request) (*url.URL, error) {
	if c.proxyURL == "" {
		return http.ProxyFromEnvironment
	}
	u, err := url.Parse(c.proxyURL)
	if err != nil {
		return http.ProxyFromEnvironment
	}
	return http.ProxyURL(u)
}
