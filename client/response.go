package client

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
)

// Response wraps a resty.Response and adds ergonomic decode helpers.
type Response struct {
	raw *resty.Response
}

func newResponse(r *resty.Response) *Response {
	return &Response{raw: r}
}

// Raw returns the underlying *resty.Response.
func (r *Response) Raw() *resty.Response { return r.raw }

// RawHTTP returns the underlying *http.Response.
func (r *Response) RawHTTP() *http.Response { return r.raw.RawResponse }

// StatusCode returns the HTTP status code.
func (r *Response) StatusCode() int { return r.raw.StatusCode() }

// Status returns the HTTP status string (e.g. "200 OK").
func (r *Response) Status() string { return r.raw.Status() }

// Header returns the value of a response header.
func (r *Response) Header(key string) string { return r.raw.Header().Get(key) }

// Headers returns all response headers.
func (r *Response) Headers() http.Header { return r.raw.Header() }

// Bytes returns the raw response body as a byte slice.
func (r *Response) Bytes() []byte { return r.raw.Body() }

// String returns the response body as a string.
func (r *Response) String() string { return r.raw.String() }

// IsSuccess returns true if the HTTP status code is in the 2xx range.
func (r *Response) IsSuccess() bool { return r.raw.IsSuccess() }

// IsError returns true if the HTTP status code is 4xx or 5xx.
func (r *Response) IsError() bool { return r.raw.IsError() }

// IsClientError returns true if the HTTP status code is 4xx.
func (r *Response) IsClientError() bool {
	code := r.raw.StatusCode()
	return code >= 400 && code < 500
}

// IsServerError returns true if the HTTP status code is 5xx.
func (r *Response) IsServerError() bool {
	code := r.raw.StatusCode()
	return code >= 500 && code < 600
}

// Decode decodes the response body into v using the Content-Type of the response.
// Supports application/json and application/xml. Falls back to JSON.
func (r *Response) Decode(v any) error {
	ct := r.Header("Content-Type")
	switch {
	case strings.Contains(ct, "application/xml"), strings.Contains(ct, "text/xml"):
		return r.XML(v)
	default:
		return r.JSON(v)
	}
}

// JSON decodes the response body as JSON into v.
func (r *Response) JSON(v any) error {
	if err := json.Unmarshal(r.Bytes(), v); err != nil {
		return fmt.Errorf("httpclient: JSON decode failed (status %d): %w", r.StatusCode(), err)
	}
	return nil
}

// XML decodes the response body as XML into v.
func (r *Response) XML(v any) error {
	if err := xml.Unmarshal(r.Bytes(), v); err != nil {
		return fmt.Errorf("httpclient: XML decode failed (status %d): %w", r.StatusCode(), err)
	}
	return nil
}

// MustJSON decodes the response as JSON or panics. Use only in tests/scripts.
func (r *Response) MustJSON(v any) {
	if err := r.JSON(v); err != nil {
		panic(err)
	}
}

// --- Error types ---

// RequestError wraps a network-level error (e.g. DNS failure, timeout).
type RequestError struct {
	Method string
	URL    string
	Err    error
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("httpclient: %s %s failed: %v", e.Method, e.URL, e.Err)
}

func (e *RequestError) Unwrap() error { return e.Err }

// HTTPError represents a non-2xx HTTP response.
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("httpclient: HTTP %s: %s", e.Status, e.Body)
}

// IsNotFound returns true if the HTTPError has status 404.
func (e *HTTPError) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsUnauthorized returns true if the HTTPError has status 401.
func (e *HTTPError) IsUnauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// IsForbidden returns true if the HTTPError has status 403.
func (e *HTTPError) IsForbidden() bool { return e.StatusCode == http.StatusForbidden }

// IsConflict returns true if the HTTPError has status 409.
func (e *HTTPError) IsConflict() bool { return e.StatusCode == http.StatusConflict }

// IsTooManyRequests returns true if the HTTPError has status 429.
func (e *HTTPError) IsTooManyRequests() bool { return e.StatusCode == http.StatusTooManyRequests }
