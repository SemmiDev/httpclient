# httpclient

A production-ready, idiomatic Go HTTP client package built on top of [go-resty/resty](https://github.com/go-resty/resty).

## Features

| Feature | Details |
|---|---|
| **Functional options** | Fluent, composable client & request configuration |
| **Generic helpers** | `DecodeJSON[T]`, `PostJSON[T]`, `PatchJSON[T]` — no type assertions |
| **Retry** | Configurable backoff, max attempts, custom retry conditions |
| **Circuit Breaker** | Closed → Open → Half-Open state machine |
| **Rate Limiting** | Token-bucket via `golang.org/x/time/rate` |
| **Middleware chain** | `http.RoundTripper` composable middleware (logging, auth, request ID, timeout) |
| **Structured logging** | `log/slog` integration with context propagation |
| **Response helpers** | `.JSON()`, `.XML()`, `.Decode()`, `.IsSuccess()`, `.IsNotFound()`, … |
| **Clone** | Per-request scoped clients (different auth, headers, etc.) |
| **TLS / Proxy** | Custom TLS config, proxy URL, or `http.ProxyFromEnvironment` |
| **Multipart upload** | `client.File(...)` for streaming file uploads |
| **Path params** | `/users/{id}` → `client.PathParam("id", "42")` |
| **Debug mode** | Full request/response dump via resty |

## Installation

```sh
go get github.com/semmidev/httpclient
```

## Quick Start

```go
import "github.com/semmidev/httpclient/client"

c := client.New(
    client.WithBaseURL("https://api.example.com"),
    client.WithBearerToken(os.Getenv("API_TOKEN")),
    client.WithTimeout(10*time.Second),
    client.WithRetry(client.RetryConfig{
        MaxAttempts: 3,
        WaitTime:    200 * time.Millisecond,
    }),
)

type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

// Generic, type-safe GET
user, resp, err := client.DecodeJSON[User](ctx, c, "/users/{id}",
    client.PathParam("id", "1"),
    client.ExpectSuccess(),
)
```

## Package Structure

```
httpclient/
├── client/
│   ├── client.go       # Client struct, HTTP methods (Get/Post/Put/Patch/Delete/…)
│   ├── options.go      # Client-level functional options (WithBaseURL, WithRetry, …)
│   ├── request.go      # Per-request options (Body, QueryParam, Bind, File, …)
│   ├── response.go     # Response wrapper, error types (HTTPError, RequestError)
│   └── generics.go     # Generic helpers: DecodeJSON[T], PostJSON[T], Must[T], …
├── middleware/
│   └── middleware.go   # RoundTripper middleware: Chain, RateLimit, CircuitBreaker, Logging, …
├── circuitbreaker/
│   └── circuitbreaker.go  # Standalone circuit breaker (Closed/Open/HalfOpen)
└── examples/
    └── examples.go     # Complete runnable usage examples
```

## Client Options

```go
client.New(
    client.WithBaseURL("https://api.example.com"),
    client.WithTimeout(15 * time.Second),
    client.WithHeader("X-App-Version", "2.0"),
    client.WithHeaders(map[string]string{"Accept": "application/json"}),
    client.WithQueryParam("api_version", "v2"),
    client.WithBearerToken("jwt-token"),
    client.WithBasicAuth("user", "pass"),
    client.WithRetry(client.RetryConfig{
        MaxAttempts: 3,
        WaitTime:    100 * time.Millisecond,
        MaxWaitTime: 2 * time.Second,
        RetryConditions: []client.RetryConditionFunc{
            func(resp *http.Response, err error) bool {
                return resp != nil && resp.StatusCode == 429
            },
        },
    }),
    client.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS13}),
    client.WithProxy("http://proxy.corp.internal:8080"),
    client.WithLogger(slog.Default()),
    client.WithDebug(false),
    client.WithFollowRedirect(true, 5),
    client.WithTransport(myCustomTransport),
)
```

## Request Options

```go
c.Get(ctx, "/users/{id}",
    client.PathParam("id", "42"),
    client.QueryParam("include", "roles"),
    client.QueryParams(map[string]string{"fields": "id,name"}),
    client.Header("X-Trace-ID", traceID),
    client.Bind(&result),       // decode response JSON into result
    client.BindError(&apiErr),  // decode error body on non-2xx
    client.ExpectSuccess(),     // return HTTPError on non-2xx
)

c.Post(ctx, "/users",
    client.Body(createReq),       // any JSON-serializable value
    client.RawBody([]byte{...}),  // raw bytes
    client.BodyReader(r),         // io.Reader
    client.FormData(map[string]string{"field": "value"}),
    client.File("avatar", "photo.jpg", fileReader),
    client.ContentType("application/json"),
    client.ForceJSON(),
)
```

## Generic Helpers

```go
// Type-safe response decoding — no manual json.Unmarshal
user,  resp, err := client.DecodeJSON[User](ctx, c, "/users/1")
users, resp, err := client.DecodeJSON[[]User](ctx, c, "/users")
ok,    resp, err := client.PostJSON[SuccessResponse](ctx, c, "/login", client.Body(creds))
feed,  resp, err := client.DecodeXML[RSSFeed](ctx, c, "/rss.xml")

// One-liners for scripts/tests (panics on error)
user := client.Must(client.DecodeJSON[User](ctx, c, "/users/1"))
```

## Middleware Chain

```go
import (
    "golang.org/x/time/rate"
    "github.com/semmidev/httpclient/middleware"
    "github.com/semmidev/httpclient/circuitbreaker"
)

cb      := circuitbreaker.New(circuitbreaker.DefaultConfig())
limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 10)

transport := middleware.Chain(
    http.DefaultTransport,           // base transport (innermost)
    middleware.RateLimit(limiter),   // applied first
    middleware.CircuitBreaker(cb),
    middleware.Logging(slog.Default()),
    middleware.UserAgent("MyService/1.0"),
    middleware.RequestID(uuid.NewString),
    middleware.Timeout(5*time.Second),
)

c := client.New(
    client.WithBaseURL("https://api.example.com"),
    client.WithTransport(transport),
)
```

## Error Handling

```go
resp, err := c.Get(ctx, "/users/1", client.ExpectSuccess())
if err != nil {
    var httpErr *client.HTTPError
    if errors.As(err, &httpErr) {
        switch {
        case httpErr.IsNotFound():      // 404
        case httpErr.IsUnauthorized():  // 401
        case httpErr.IsForbidden():     // 403
        case httpErr.IsConflict():      // 409
        case httpErr.IsTooManyRequests(): // 429
        default:
            fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Body)
        }
        return
    }
    var reqErr *client.RequestError  // network error
    if errors.As(err, &reqErr) { ... }
}
```

## Clone for Multi-Tenant / Per-User Auth

```go
base := client.New(
    client.WithBaseURL("https://api.example.com"),
    client.WithTimeout(10*time.Second),
)

// Each request handler creates a scoped client — zero allocation of duplicate state
userClient := base.Clone(client.WithBearerToken(getUserJWT(ctx)))
user, _, _ := client.DecodeJSON[User](ctx, userClient, "/me")
```

## Circuit Breaker

```go
cb := circuitbreaker.New(circuitbreaker.Config{
    MaxFailures:       5,
    ResetTimeout:      30 * time.Second,
    HalfOpenSuccesses: 2,
    ShouldTrip: func(resp *http.Response, err error) bool {
        return err != nil || (resp != nil && resp.StatusCode >= 500)
    },
})

fmt.Println(cb.State()) // "closed" | "open" | "half-open"
cb.Reset()              // manually reset
```

## Testing

Use `httptest.Server` to test your client without real network calls:

```go
func TestGetUser(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(User{ID: 1, Name: "Budi"})
    }))
    defer ts.Close()

    c := client.New(client.WithBaseURL(ts.URL))
    user, _, err := client.DecodeJSON[User](context.Background(), c, "/users/1")
    require.NoError(t, err)
    assert.Equal(t, "Budi", user.Name)
}
```

## Dependencies

- [`github.com/go-resty/resty/v2`](https://github.com/go-resty/resty) — HTTP client core
- [`golang.org/x/time`](https://pkg.go.dev/golang.org/x/time/rate) — rate limiter
