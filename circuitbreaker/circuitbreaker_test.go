package circuitbreaker_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/semmidev/httpclient/circuitbreaker"
)

func TestCircuitBreaker_StartsClose(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("expected Closed, got %s", cb.State())
	}
	if err := cb.Allow(); err != nil {
		t.Errorf("expected Allow() = nil, got %v", err)
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig()
	cfg.MaxFailures = 3
	cb := circuitbreaker.New(cfg)

	for i := 0; i < 3; i++ {
		_ = cb.Allow()
		cb.Record(nil, errors.New("fail"))
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Errorf("expected Open, got %s", cb.State())
	}
	if err := cb.Allow(); !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Errorf("expected ErrOpen, got %v", err)
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig()
	cfg.MaxFailures = 1
	cfg.ResetTimeout = 10 * time.Millisecond
	cb := circuitbreaker.New(cfg)

	_ = cb.Allow()
	cb.Record(nil, errors.New("fail"))

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected Open")
	}

	time.Sleep(20 * time.Millisecond)

	if err := cb.Allow(); err != nil {
		t.Errorf("expected Allow() after reset, got %v", err)
	}
	if cb.State() != circuitbreaker.StateHalfOpen {
		t.Errorf("expected HalfOpen, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterHalfOpenSuccesses(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig()
	cfg.MaxFailures = 1
	cfg.ResetTimeout = 5 * time.Millisecond
	cfg.HalfOpenSuccesses = 2
	cb := circuitbreaker.New(cfg)

	// Trip the breaker
	_ = cb.Allow()
	cb.Record(nil, errors.New("fail"))
	time.Sleep(10 * time.Millisecond)

	// Two successful requests in half-open
	for i := 0; i < 2; i++ {
		_ = cb.Allow()
		ok := &http.Response{StatusCode: 200}
		cb.Record(ok, nil)
	}

	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("expected Closed after half-open successes, got %s", cb.State())
	}
}

func TestCircuitBreaker_ReopensOnHalfOpenFailure(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig()
	cfg.MaxFailures = 1
	cfg.ResetTimeout = 5 * time.Millisecond
	cb := circuitbreaker.New(cfg)

	_ = cb.Allow()
	cb.Record(nil, errors.New("fail"))
	time.Sleep(10 * time.Millisecond)

	// Fail again in half-open
	_ = cb.Allow()
	cb.Record(nil, errors.New("still failing"))

	if cb.State() != circuitbreaker.StateOpen {
		t.Errorf("expected Open after half-open failure, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig()
	cfg.MaxFailures = 1
	cb := circuitbreaker.New(cfg)

	_ = cb.Allow()
	cb.Record(nil, errors.New("fail"))

	cb.Reset()

	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("expected Closed after Reset, got %s", cb.State())
	}
	if err := cb.Allow(); err != nil {
		t.Errorf("expected Allow() after Reset, got %v", err)
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig()
	cfg.MaxFailures = 3
	cb := circuitbreaker.New(cfg)

	// 2 failures, then 1 success — should stay closed
	_ = cb.Allow()
	cb.Record(nil, errors.New("fail"))
	_ = cb.Allow()
	cb.Record(nil, errors.New("fail"))
	_ = cb.Allow()
	cb.Record(&http.Response{StatusCode: 200}, nil) // resets counter

	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("expected Closed after success reset, got %s", cb.State())
	}
}
