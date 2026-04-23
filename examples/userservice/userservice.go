// Package userservice demonstrates the recommended service-layer pattern:
// wrap httpclient into a typed, domain-specific API client so HTTP details
// stay out of business logic, only domain types are exposed, and errors
// are translated at the boundary.
package userservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/semmidev/httpclient/client"
	"github.com/semmidev/httpclient/pagination"
)

// --- Domain types ---

type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type UpdateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
	Role  *string `json:"role,omitempty"`
}

type ListUsersResponse struct {
	Data       []User `json:"data"`
	TotalPages int    `json:"total_pages"`
	TotalItems int    `json:"total_items"`
}

type ListUsersFilter struct {
	Role   string
	Search string
	Page   int
	Limit  int
}

// --- Sentinel errors ---

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailTaken   = errors.New("email already taken")
)

// --- Service ---

// Service provides typed methods for the Users API.
// Create once per application and share across handlers.
type Service struct {
	c *client.Client
}

// New constructs a Service backed by the given client.
func New(c *client.Client) *Service {
	return &Service{c: c}
}

// Get retrieves a single user by ID.
func (s *Service) Get(ctx context.Context, id int) (User, error) {
	user, _, err := client.DecodeJSON[User](ctx, s.c, "/users/{id}",
		client.PathParam("id", fmt.Sprintf("%d", id)),
		client.ExpectSuccess(),
	)
	if err != nil {
		return User{}, s.mapErr(fmt.Sprintf("Get(%d)", id), err)
	}
	return user, nil
}

// List retrieves a paginated, filtered list of users.
func (s *Service) List(ctx context.Context, f ListUsersFilter) (ListUsersResponse, error) {
	opts := []client.RequestOption{client.ExpectSuccess()}
	if f.Role != "" {
		opts = append(opts, client.QueryParam("role", f.Role))
	}
	if f.Search != "" {
		opts = append(opts, client.QueryParam("q", f.Search))
	}
	if f.Page > 0 {
		opts = append(opts, client.QueryParam("page", fmt.Sprintf("%d", f.Page)))
	}
	if f.Limit > 0 {
		opts = append(opts, client.QueryParam("limit", fmt.Sprintf("%d", f.Limit)))
	}

	result, _, err := client.DecodeJSON[ListUsersResponse](ctx, s.c, "/users", opts...)
	if err != nil {
		return ListUsersResponse{}, fmt.Errorf("userservice.List: %w", err)
	}
	return result, nil
}

// ListAll fetches every user across all pages.
// Use sparingly — prefer List for UI-bound queries.
func (s *Service) ListAll(ctx context.Context) ([]User, error) {
	users, err := pagination.FetchAllPages(
		ctx, s.c, "/users",
		pagination.OffsetConfig{PageSize: 100},
		func(_ *client.Response, body []byte) (pagination.PageResult[User], error) {
			var r ListUsersResponse
			if err := json.Unmarshal(body, &r); err != nil {
				return pagination.PageResult[User]{}, err
			}
			return pagination.PageResult[User]{
				Items:      r.Data,
				TotalPages: r.TotalPages,
				HasMore:    len(r.Data) > 0,
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("userservice.ListAll: %w", err)
	}
	return users, nil
}

// Create creates a new user, returning ErrEmailTaken on conflict.
func (s *Service) Create(ctx context.Context, req CreateUserRequest) (User, error) {
	user, _, err := client.PostJSON[User](ctx, s.c, "/users",
		client.Body(req),
		client.ExpectSuccess(),
	)
	if err != nil {
		return User{}, s.mapErr("Create", err)
	}
	return user, nil
}

// Update partially updates a user. Nil fields are omitted from the payload.
func (s *Service) Update(ctx context.Context, id int, req UpdateUserRequest) (User, error) {
	user, _, err := client.PatchJSON[User](ctx, s.c, "/users/{id}",
		client.PathParam("id", fmt.Sprintf("%d", id)),
		client.Body(req),
		client.ExpectSuccess(),
	)
	if err != nil {
		return User{}, s.mapErr(fmt.Sprintf("Update(%d)", id), err)
	}
	return user, nil
}

// Delete deletes a user by ID.
func (s *Service) Delete(ctx context.Context, id int) error {
	_, err := s.c.Delete(ctx, "/users/{id}",
		client.PathParam("id", fmt.Sprintf("%d", id)),
		client.ExpectSuccess(),
	)
	if err != nil {
		return s.mapErr(fmt.Sprintf("Delete(%d)", id), err)
	}
	return nil
}

// UploadAvatar uploads a profile picture for the user.
func (s *Service) UploadAvatar(ctx context.Context, id int, filename string, data []byte) error {
	_, err := s.c.Post(ctx, "/users/{id}/avatar",
		client.PathParam("id", fmt.Sprintf("%d", id)),
		client.File("avatar", filename, bytes.NewReader(data)),
		client.ExpectSuccess(),
	)
	if err != nil {
		return fmt.Errorf("userservice.UploadAvatar(%d): %w", id, err)
	}
	return nil
}

// mapErr translates HTTP status codes into domain-level sentinel errors.
func (s *Service) mapErr(op string, err error) error {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.IsNotFound():
			return fmt.Errorf("userservice.%s: %w", op, ErrUserNotFound)
		case httpErr.IsConflict():
			return fmt.Errorf("userservice.%s: %w", op, ErrEmailTaken)
		}
	}
	return fmt.Errorf("userservice.%s: %w", op, err)
}
