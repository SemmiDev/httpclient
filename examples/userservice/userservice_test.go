package userservice_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/semmidev/httpclient/client"
	"github.com/semmidev/httpclient/examples/userservice"
	"github.com/semmidev/httpclient/mock"
)

func newService(t *testing.T) (*userservice.Service, *mock.Transport) {
	t.Helper()
	tr := mock.NewTransport(t)
	c := client.New(
		client.WithBaseURL("https://api.test"),
		client.WithTransport(tr),
	)
	return userservice.New(c), tr
}

func TestService_Get_Success(t *testing.T) {
	svc, tr := newService(t)

	tr.On(http.MethodGet, "/users/1", func(_ *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusOK, userservice.User{ID: 1, Name: "Budi"})
	})

	user, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != 1 || user.Name != "Budi" {
		t.Errorf("unexpected user: %+v", user)
	}
	tr.AssertCalled(http.MethodGet, "/users/1")
}

func TestService_Get_NotFound(t *testing.T) {
	svc, tr := newService(t)

	tr.On(http.MethodGet, "/users/999", func(_ *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusNotFound, map[string]string{"error": "not found"})
	})

	_, err := svc.Get(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, userservice.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestService_Create_Success(t *testing.T) {
	svc, tr := newService(t)

	tr.On(http.MethodPost, "/users", func(_ *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusCreated, userservice.User{ID: 42, Name: "Sari", Email: "sari@example.com"})
	})

	user, err := svc.Create(context.Background(), userservice.CreateUserRequest{
		Name:  "Sari",
		Email: "sari@example.com",
		Role:  "member",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != 42 {
		t.Errorf("expected ID=42, got %d", user.ID)
	}
}

func TestService_Create_EmailTaken(t *testing.T) {
	svc, tr := newService(t)

	tr.On(http.MethodPost, "/users", func(_ *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusConflict, map[string]string{"error": "email taken"})
	})

	_, err := svc.Create(context.Background(), userservice.CreateUserRequest{
		Name:  "Duplikat",
		Email: "duplikat@example.com",
	})
	if !errors.Is(err, userservice.ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got: %v", err)
	}
}

func TestService_Delete_Success(t *testing.T) {
	svc, tr := newService(t)

	tr.On(http.MethodDelete, "/users/5", func(_ *http.Request) (*http.Response, error) {
		return mock.EmptyResponse(http.StatusNoContent)
	})

	err := svc.Delete(context.Background(), 5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	tr.AssertCallCount(1)
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, tr := newService(t)

	tr.On(http.MethodDelete, "/users/99", func(_ *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusNotFound, map[string]string{"error": "not found"})
	})

	err := svc.Delete(context.Background(), 99)
	if !errors.Is(err, userservice.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestService_Update(t *testing.T) {
	svc, tr := newService(t)

	newName := "Budi Updated"
	tr.On(http.MethodPatch, "/users/1", func(r *http.Request) (*http.Response, error) {
		return mock.JSONResponse(http.StatusOK, userservice.User{ID: 1, Name: newName})
	})

	user, err := svc.Update(context.Background(), 1, userservice.UpdateUserRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != newName {
		t.Errorf("expected Name=%q, got %q", newName, user.Name)
	}
}
