package client

import (
	"context"
	"encoding/json"
	"encoding/xml"
)

// DecodeJSON is a generic helper that performs a GET and decodes the JSON response into T.
// Ideal for simple JSON APIs.
//
//	users, resp, err := client.DecodeJSON[[]User](ctx, c, "/users")
func DecodeJSON[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (T, *Response, error) {
	var zero T
	resp, err := c.Get(ctx, path, opts...)
	if err != nil {
		return zero, resp, err
	}
	var out T
	if err := resp.JSON(&out); err != nil {
		return zero, resp, err
	}
	return out, resp, nil
}

// PostJSON sends a POST with a JSON body and decodes the response into T.
//
//	created, resp, err := client.PostJSON[User](ctx, c, "/users", Body(payload))
func PostJSON[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (T, *Response, error) {
	var zero T
	resp, err := c.Post(ctx, path, opts...)
	if err != nil {
		return zero, resp, err
	}
	var out T
	if err := resp.JSON(&out); err != nil {
		return zero, resp, err
	}
	return out, resp, nil
}

// PutJSON sends a PUT with a JSON body and decodes the response into T.
func PutJSON[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (T, *Response, error) {
	var zero T
	resp, err := c.Put(ctx, path, opts...)
	if err != nil {
		return zero, resp, err
	}
	var out T
	if err := resp.JSON(&out); err != nil {
		return zero, resp, err
	}
	return out, resp, nil
}

// PatchJSON sends a PATCH with a JSON body and decodes the response into T.
func PatchJSON[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (T, *Response, error) {
	var zero T
	resp, err := c.Patch(ctx, path, opts...)
	if err != nil {
		return zero, resp, err
	}
	var out T
	if err := resp.JSON(&out); err != nil {
		return zero, resp, err
	}
	return out, resp, nil
}

// DecodeXML is a generic helper that performs a GET and decodes the XML response into T.
func DecodeXML[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (T, *Response, error) {
	var zero T
	resp, err := c.Get(ctx, path, opts...)
	if err != nil {
		return zero, resp, err
	}
	var out T
	if err := resp.XML(&out); err != nil {
		return zero, resp, err
	}
	return out, resp, nil
}

// Must is a helper for test/dev usage: panics on non-nil error.
// Do NOT use in production code.
func Must[T any](v T, _ *Response, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// ParseJSON is a one-shot utility to decode a JSON byte slice into T.
func ParseJSON[T any](b []byte) (T, error) {
	var out T
	return out, json.Unmarshal(b, &out)
}

// ParseXML is a one-shot utility to decode an XML byte slice into T.
func ParseXML[T any](b []byte) (T, error) {
	var out T
	return out, xml.Unmarshal(b, &out)
}
