// Package examples demonstrates idiomatic usage of the httpclient package.
package examples

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"golang.org/x/time/rate"

	"github.com/semmidev/httpclient/circuitbreaker"
	"github.com/semmidev/httpclient/client"
	"github.com/semmidev/httpclient/middleware"
)

// --- Domain Types ---

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// =============================================================================
// Example 1: Basic JSON API Client
// =============================================================================

func Example_basicUsage() {
	ctx := context.Background()

	c := client.New(
		client.WithBaseURL("https://jsonplaceholder.typicode.com"),
		client.WithTimeout(10*time.Second),
		client.WithHeader("Accept", "application/json"),
	)

	// GET with generic helper — clean and type-safe
	user, resp, err := client.DecodeJSON[User](ctx, c, "/users/1")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Status: %d | User: %+v\n", resp.StatusCode(), user)
}

// =============================================================================
// Example 2: POST with JSON body
// =============================================================================

func Example_postJSON() {
	ctx := context.Background()

	c := client.New(
		client.WithBaseURL("https://jsonplaceholder.typicode.com"),
		client.WithBearerToken("my-secret-token"),
	)

	payload := CreateUserRequest{Name: "Budi", Email: "budi@example.com"}

	created, resp, err := client.PostJSON[User](ctx, c, "/users",
		client.Body(payload),
		client.ExpectSuccess(), // returns HTTPError if non-2xx
	)
	if err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Body)
		}
		return
	}
	fmt.Printf("Created user %d via %s\n", created.ID, resp.Header("Content-Type"))
}

// =============================================================================
// Example 3: Path params, query params, and Bind
// =============================================================================

func Example_paramsAndBind() {
	ctx := context.Background()

	c := client.New(client.WithBaseURL("https://api.example.com"))

	var users []User
	resp, err := c.Get(ctx, "/orgs/{org}/members",
		client.PathParam("org", "my-org"),
		client.QueryParams(map[string]string{
			"page":     "1",
			"per_page": "20",
		}),
		client.Bind(&users), // auto-decode on success
		client.ExpectSuccess(),
	)
	if err != nil {
		return
	}
	fmt.Printf("Got %d users, next page header: %s\n", len(users), resp.Header("X-Next-Page"))
}

// =============================================================================
// Example 4: File upload (multipart)
// =============================================================================

func Example_fileUpload() {
	ctx := context.Background()

	c := client.New(client.WithBaseURL("https://api.example.com"))

	fileContent := bytes.NewBufferString("hello world file content")

	resp, err := c.Post(ctx, "/upload",
		client.File("file", "hello.txt", fileContent),
		client.FormData(map[string]string{"description": "Test upload"}),
	)
	if err != nil {
		return
	}
	fmt.Println("Upload status:", resp.StatusCode())
}

// =============================================================================
// Example 5: Retry + Circuit Breaker + Rate Limiter
// =============================================================================

func Example_resilientClient() {
	ctx := context.Background()

	// Circuit breaker: open after 5 failures, recover after 30s
	cb := circuitbreaker.New(circuitbreaker.Config{
		MaxFailures:       5,
		ResetTimeout:      30 * time.Second,
		HalfOpenSuccesses: 2,
		ShouldTrip:        circuitbreaker.DefaultConfig().ShouldTrip,
	})

	// Rate limiter: 10 requests/second, burst of 5
	limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 5)

	// Build a custom transport with middleware chain
	transport := middleware.Chain(
		http.DefaultTransport,
		middleware.RateLimit(limiter),
		middleware.CircuitBreaker(cb),
		middleware.Logging(slog.Default()),
		middleware.UserAgent("MyService/1.0"),
	)

	c := client.New(
		client.WithBaseURL("https://api.example.com"),
		client.WithTransport(transport),
		client.WithRetry(client.RetryConfig{
			MaxAttempts: 3,
			WaitTime:    200 * time.Millisecond,
			MaxWaitTime: 2 * time.Second,
			// Retry on 429 Too Many Requests as well
			RetryConditions: []client.RetryConditionFunc{
				func(resp *http.Response, err error) bool {
					return resp != nil && resp.StatusCode == http.StatusTooManyRequests
				},
			},
		}),
	)

	users, _, err := client.DecodeJSON[[]User](ctx, c, "/users")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Fetched %d users (circuit: %s)\n", len(users), cb.State())
}

// =============================================================================
// Example 6: Per-request client (clone with different auth)
// =============================================================================

func Example_perRequestToken() {
	ctx := context.Background()

	// Base client shared across the app
	base := client.New(
		client.WithBaseURL("https://api.example.com"),
		client.WithTimeout(10*time.Second),
	)

	// Per-user client with a different bearer token
	userClient := base.Clone(
		client.WithBearerToken(getUserToken(ctx)),
	)

	user, _, _ := client.DecodeJSON[User](ctx, userClient, "/me")
	fmt.Println("Hello,", user.Name)
}

func getUserToken(_ context.Context) string { return "user-specific-jwt" }

// =============================================================================
// Example 7: Structured logging and debug mode
// =============================================================================

func Example_logging() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	c := client.New(
		client.WithBaseURL("https://api.example.com"),
		client.WithLogger(logger),
		client.WithDebug(true), // resty debug output
	)

	resp, err := c.Get(context.Background(), "/health")
	if err != nil {
		return
	}
	fmt.Println("Health:", resp.String())
}

// =============================================================================
// Example 8: Error handling patterns
// =============================================================================

func Example_errorHandling() {
	ctx := context.Background()
	c := client.New(client.WithBaseURL("https://api.example.com"))

	resp, err := c.Get(ctx, "/users/999", client.ExpectSuccess())
	if err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			switch {
			case httpErr.IsNotFound():
				fmt.Println("user not found")
			case httpErr.IsUnauthorized():
				fmt.Println("need to login")
			case httpErr.IsTooManyRequests():
				fmt.Println("slow down!")
			default:
				fmt.Printf("HTTP error %d: %s\n", httpErr.StatusCode, httpErr.Body)
			}
			return
		}
		var reqErr *client.RequestError
		if errors.As(err, &reqErr) {
			fmt.Println("network error:", reqErr)
			return
		}
	}
	_ = resp
}

// =============================================================================
// Example 9: Convenience — Must (only for tests/scripts)
// =============================================================================

func Example_mustHelper() {
	ctx := context.Background()
	c := client.New(client.WithBaseURL("https://jsonplaceholder.typicode.com"))

	// Panics on error — use only in tests or one-off scripts
	user := client.Must(client.DecodeJSON[User](ctx, c, "/users/1"))
	fmt.Println(user.Name)
}

// =============================================================================
// Example 10: XML API
// =============================================================================

type RSSFeed struct {
	Title string `xml:"channel>title"`
}

func Example_xmlAPI() {
	ctx := context.Background()
	c := client.New(client.WithBaseURL("https://feeds.example.com"))

	feed, _, err := client.DecodeXML[RSSFeed](ctx, c, "/rss")
	if err != nil {
		return
	}
	fmt.Println("Feed:", feed.Title)
}
