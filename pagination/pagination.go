// Package pagination provides generic helpers for consuming paginated HTTP APIs.
// Supports both offset/page-based and cursor-based pagination patterns.
package pagination

import (
	"context"
	"fmt"
	"strconv"

	"github.com/semmidev/httpclient/client"
)

// PageResult holds a single page of results from a paginated API.
type PageResult[T any] struct {
	Items      []T
	Page       int
	TotalPages int
	TotalItems int
	HasMore    bool
	NextCursor string
}

// PageExtractor extracts pagination metadata from a response.
// Implement this for your specific API contract.
type PageExtractor[T any] func(resp *client.Response, body []byte) (PageResult[T], error)

// OffsetConfig configures offset/page-based pagination.
type OffsetConfig struct {
	// PageParam is the query parameter name for the page number (default: "page").
	PageParam string
	// SizeParam is the query parameter name for page size (default: "per_page").
	SizeParam string
	// PageSize is the number of items per page (default: 100).
	PageSize int
	// StartPage is the first page number (default: 1, some APIs use 0).
	StartPage int
	// MaxPages limits total pages fetched. 0 = no limit.
	MaxPages int
}

func (c *OffsetConfig) defaults() {
	if c.PageParam == "" {
		c.PageParam = "page"
	}
	if c.SizeParam == "" {
		c.SizeParam = "per_page"
	}
	if c.PageSize == 0 {
		c.PageSize = 100
	}
	if c.StartPage == 0 {
		c.StartPage = 1
	}
}

// FetchAllPages fetches all pages of a paginated offset-based API and returns
// every item as a flat slice. The extractor function decodes each page response.
//
//	type APIResponse struct {
//	    Data       []User `json:"data"`
//	    TotalPages int    `json:"total_pages"`
//	}
//
//	users, err := pagination.FetchAllPages(ctx, c, "/users",
//	    pagination.OffsetConfig{PageSize: 50},
//	    func(resp *client.Response, body []byte) (pagination.PageResult[User], error) {
//	        var api APIResponse
//	        if err := json.Unmarshal(body, &api); err != nil { return pagination.PageResult[User]{}, err }
//	        return pagination.PageResult[User]{
//	            Items:      api.Data,
//	            TotalPages: api.TotalPages,
//	            HasMore:    resp.Header("X-Next-Page") != "",
//	        }, nil
//	    },
//	)
func FetchAllPages[T any](
	ctx context.Context,
	c *client.Client,
	path string,
	cfg OffsetConfig,
	extractor PageExtractor[T],
	extraOpts ...client.RequestOption,
) ([]T, error) {
	cfg.defaults()

	var all []T
	page := cfg.StartPage

	for {
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		default:
		}

		opts := append([]client.RequestOption{
			client.QueryParam(cfg.PageParam, strconv.Itoa(page)),
			client.QueryParam(cfg.SizeParam, strconv.Itoa(cfg.PageSize)),
		}, extraOpts...)

		resp, err := c.Get(ctx, path, opts...)
		if err != nil {
			return all, fmt.Errorf("pagination: page %d: %w", page, err)
		}

		result, err := extractor(resp, resp.Bytes())
		if err != nil {
			return all, fmt.Errorf("pagination: extract page %d: %w", page, err)
		}

		all = append(all, result.Items...)

		if !result.HasMore {
			break
		}
		if cfg.MaxPages > 0 && page-cfg.StartPage+1 >= cfg.MaxPages {
			break
		}

		page++
	}

	return all, nil
}

// CursorConfig configures cursor-based pagination.
type CursorConfig struct {
	// CursorParam is the query parameter name for the cursor (default: "cursor").
	CursorParam string
	// SizeParam is the query parameter for page size (default: "limit").
	SizeParam string
	// PageSize is the number of items per page (default: 100).
	PageSize int
	// MaxPages limits total pages. 0 = no limit.
	MaxPages int
}

func (c *CursorConfig) defaults() {
	if c.CursorParam == "" {
		c.CursorParam = "cursor"
	}
	if c.SizeParam == "" {
		c.SizeParam = "limit"
	}
	if c.PageSize == 0 {
		c.PageSize = 100
	}
}

// FetchAllCursorPages fetches all pages of a cursor-based paginated API.
//
//	items, err := pagination.FetchAllCursorPages(ctx, c, "/events",
//	    pagination.CursorConfig{PageSize: 50},
//	    func(resp *client.Response, body []byte) (pagination.PageResult[Event], error) {
//	        var api struct {
//	            Data       []Event `json:"data"`
//	            NextCursor string  `json:"next_cursor"`
//	        }
//	        if err := json.Unmarshal(body, &api); err != nil { return pagination.PageResult[Event]{}, err }
//	        return pagination.PageResult[Event]{
//	            Items:      api.Data,
//	            NextCursor: api.NextCursor,
//	            HasMore:    api.NextCursor != "",
//	        }, nil
//	    },
//	)
func FetchAllCursorPages[T any](
	ctx context.Context,
	c *client.Client,
	path string,
	cfg CursorConfig,
	extractor PageExtractor[T],
	extraOpts ...client.RequestOption,
) ([]T, error) {
	cfg.defaults()

	var (
		all    []T
		cursor string
		page   int
	)

	for {
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		default:
		}

		opts := []client.RequestOption{
			client.QueryParam(cfg.SizeParam, strconv.Itoa(cfg.PageSize)),
		}
		if cursor != "" {
			opts = append(opts, client.QueryParam(cfg.CursorParam, cursor))
		}
		opts = append(opts, extraOpts...)

		resp, err := c.Get(ctx, path, opts...)
		if err != nil {
			return all, fmt.Errorf("pagination: cursor page %d: %w", page+1, err)
		}

		result, err := extractor(resp, resp.Bytes())
		if err != nil {
			return all, fmt.Errorf("pagination: extract cursor page %d: %w", page+1, err)
		}

		all = append(all, result.Items...)
		page++

		if !result.HasMore || result.NextCursor == "" {
			break
		}
		if cfg.MaxPages > 0 && page >= cfg.MaxPages {
			break
		}

		cursor = result.NextCursor
	}

	return all, nil
}

// ForEachPage calls fn for each page, allowing early exit by returning false.
// Useful for streaming-style processing without accumulating all items in memory.
func ForEachPage[T any](
	ctx context.Context,
	c *client.Client,
	path string,
	cfg OffsetConfig,
	extractor PageExtractor[T],
	fn func(page int, items []T) (continueNext bool),
	extraOpts ...client.RequestOption,
) error {
	cfg.defaults()
	page := cfg.StartPage

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		opts := append([]client.RequestOption{
			client.QueryParam(cfg.PageParam, strconv.Itoa(page)),
			client.QueryParam(cfg.SizeParam, strconv.Itoa(cfg.PageSize)),
		}, extraOpts...)

		resp, err := c.Get(ctx, path, opts...)
		if err != nil {
			return fmt.Errorf("pagination: page %d: %w", page, err)
		}

		result, err := extractor(resp, resp.Bytes())
		if err != nil {
			return fmt.Errorf("pagination: extract page %d: %w", page, err)
		}

		if !fn(page-cfg.StartPage+1, result.Items) {
			return nil
		}
		if !result.HasMore {
			return nil
		}
		if cfg.MaxPages > 0 && page-cfg.StartPage+1 >= cfg.MaxPages {
			return nil
		}

		page++
	}
}
