package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/semmidev/httpclient/client"
	"github.com/semmidev/httpclient/mock"
)

// newTestServer is a helper that spins up a real httptest.Server.
func newTestServer(t *testing.T, handler http.Handler) (*httptest.Server, *client.Client) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	c := client.New(client.WithBaseURL(ts.URL))
	return ts, c
}

// --- GET / JSON ---

func TestGet_JSON(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"name":"Budi"}`))
	}))

	user, resp, err := client.DecodeJSON[User](context.Background(), c, "/users/1")

	assertNoError(t, err)
	assertEqual(t, 200, resp.StatusCode())
	assertEqual(t, 1, user.ID)
	assertEqual(t, "Budi", user.Name)
}

func TestGet_QueryParams(t *testing.T) {
	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertEqual(t, "10", r.URL.Query().Get("limit"))
		assertEqual(t, "2", r.URL.Query().Get("page"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))

	_, err := c.Get(context.Background(), "/users",
		client.QueryParams(map[string]string{"limit": "10", "page": "2"}),
	)
	assertNoError(t, err)
}

func TestGet_PathParams(t *testing.T) {
	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertEqual(t, "/users/42", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))

	_, err := c.Get(context.Background(), "/users/{id}",
		client.PathParam("id", "42"),
	)
	assertNoError(t, err)
}

// --- POST ---

func TestPost_JSONBody(t *testing.T) {
	type Req struct{ Name string }
	type Resp struct{ ID int }

	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertEqual(t, http.MethodPost, r.Method)
		assertEqual(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":99}`))
	}))

	result, resp, err := client.PostJSON[Resp](context.Background(), c, "/users",
		client.Body(Req{Name: "Budi"}),
	)

	assertNoError(t, err)
	assertEqual(t, 201, resp.StatusCode())
	assertEqual(t, 99, result.ID)
}

// --- Headers ---

func TestCustomHeader(t *testing.T) {
	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertEqual(t, "trace-abc", r.Header.Get("X-Trace-ID"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))

	_, err := c.Get(context.Background(), "/ping",
		client.Header("X-Trace-ID", "trace-abc"),
	)
	assertNoError(t, err)
}

func TestClientDefaultHeader(t *testing.T) {
	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertEqual(t, "MyApp/2.0", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))

	// Re-create client with custom default header
	c2 := c.Clone(client.WithHeader("User-Agent", "MyApp/2.0"))
	_, err := c2.Get(context.Background(), "/ping")
	assertNoError(t, err)
}

// --- ExpectSuccess + HTTPError ---

func TestExpectSuccess_ReturnsHTTPError(t *testing.T) {
	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))

	_, err := c.Get(context.Background(), "/users/999", client.ExpectSuccess())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if !httpErr.IsNotFound() {
		t.Errorf("expected IsNotFound(), got status %d", httpErr.StatusCode)
	}
}

func TestHTTPError_StatusHelpers(t *testing.T) {
	cases := []struct {
		code  int
		check func(*client.HTTPError) bool
		label string
	}{
		{401, func(e *client.HTTPError) bool { return e.IsUnauthorized() }, "IsUnauthorized"},
		{403, func(e *client.HTTPError) bool { return e.IsForbidden() }, "IsForbidden"},
		{409, func(e *client.HTTPError) bool { return e.IsConflict() }, "IsConflict"},
		{429, func(e *client.HTTPError) bool { return e.IsTooManyRequests() }, "IsTooManyRequests"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(`{}`))
			}))
			_, err := c.Get(context.Background(), "/test", client.ExpectSuccess())
			var httpErr *client.HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected HTTPError")
			}
			if !tc.check(httpErr) {
				t.Errorf("%s returned false for status %d", tc.label, tc.code)
			}
		})
	}
}

// --- Bind ---

func TestBind_AutoDecode(t *testing.T) {
	type Item struct{ Value string }

	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value":"hello"}`))
	}))

	var item Item
	_, err := c.Get(context.Background(), "/item", client.Bind(&item))

	assertNoError(t, err)
	assertEqual(t, "hello", item.Value)
}

// --- Timeout ---

func TestTimeout_ContextCancelled(t *testing.T) {
	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server
		select {
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))

	c2 := c.Clone(client.WithTimeout(50 * time.Millisecond))
	_, err := c2.Get(context.Background(), "/slow")

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// --- Clone ---

func TestClone_IndependentAuth(t *testing.T) {
	tokens := make(chan string, 2)

	_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))

	c1 := c.Clone(client.WithBearerToken("token-A"))
	c2 := c.Clone(client.WithBearerToken("token-B"))

	ctx := context.Background()
	_, _ = c1.Get(ctx, "/me")
	_, _ = c2.Get(ctx, "/me")

	got1 := <-tokens
	got2 := <-tokens

	if got1 == got2 {
		t.Errorf("expected different tokens, both got %q", got1)
	}
}

// --- Mock transport ---

func TestMockTransport(t *testing.T) {
	type User struct {
		ID int `json:"id"`
	}

	tr := mock.NewTransport(t)
	tr.On(http.MethodGet, "/users/1", func(_ *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusOK, User{ID: 1})
	})

	c := client.New(
		client.WithBaseURL("https://fake.api"),
		client.WithTransport(tr),
	)

	user, resp, err := client.DecodeJSON[User](context.Background(), c, "/users/1")
	assertNoError(t, err)
	assertEqual(t, 200, resp.StatusCode())
	assertEqual(t, 1, user.ID)

	tr.AssertCalled(http.MethodGet, "/users/1")
	tr.AssertCallCount(1)
}

func TestMockTransport_NetworkError(t *testing.T) {
	tr := mock.NewTransport(t)
	tr.On(http.MethodGet, "/unstable", func(_ *http.Request) (*http.Response, error) {
		return mock.ErrorResponse(errors.New("connection refused"))
	})

	c := client.New(
		client.WithBaseURL("https://fake.api"),
		client.WithTransport(tr),
	)

	_, err := c.Get(context.Background(), "/unstable")
	if err == nil {
		t.Fatal("expected network error")
	}
	var reqErr *client.RequestError
	if !errors.As(err, &reqErr) {
		t.Errorf("expected *RequestError, got %T", err)
	}
}

// --- Generics ---

func TestMust_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from Must on error")
		}
	}()

	tr := mock.NewTransport(t)
	tr.On(http.MethodGet, "/fail", func(_ *http.Request) (*http.Response, error) {
		return mock.ErrorResponse(errors.New("boom"))
	})

	c := client.New(
		client.WithBaseURL("https://fake.api"),
		client.WithTransport(tr),
	)

	type R struct{}
	_ = client.Must(client.DecodeJSON[R](context.Background(), c, "/fail"))
}

// --- DELETE / PUT / PATCH ---

func TestHTTPMethods(t *testing.T) {
	methods := []struct {
		name   string
		do     func(*client.Client, context.Context, string) (*client.Response, error)
		expect string
	}{
		{"PUT", func(c *client.Client, ctx context.Context, path string) (*client.Response, error) {
			return c.Put(ctx, path)
		}, http.MethodPut},
		{"PATCH", func(c *client.Client, ctx context.Context, path string) (*client.Response, error) {
			return c.Patch(ctx, path)
		}, http.MethodPatch},
		{"DELETE", func(c *client.Client, ctx context.Context, path string) (*client.Response, error) {
			return c.Delete(ctx, path)
		}, http.MethodDelete},
	}

	for _, m := range methods {
		m := m
		t.Run(m.name, func(t *testing.T) {
			_, c := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertEqual(t, m.expect, r.Method)
				w.WriteHeader(http.StatusNoContent)
			}))
			resp, err := m.do(c, context.Background(), "/resource/1")
			assertNoError(t, err)
			assertEqual(t, 204, resp.StatusCode())
		})
	}
}

// --- Test helpers ---

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("expected %v, got %v", want, got)
	}
}
