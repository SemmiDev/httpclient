// Package circuitbreaker provides a simple circuit breaker for HTTP clients.
package circuitbreaker

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

// ErrOpen is returned when the circuit is in the open state.
var ErrOpen = errors.New("circuit breaker: circuit is open")

// State represents the circuit breaker state.
type State int

const (
	StateClosed   State = iota // Normal operation
	StateHalfOpen              // Testing if service recovered
	StateOpen                  // Blocking requests
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration.
type Config struct {
	// MaxFailures is the number of consecutive failures before opening the circuit.
	MaxFailures int
	// ResetTimeout is how long to wait before transitioning to half-open.
	ResetTimeout time.Duration
	// HalfOpenSuccesses is the number of consecutive successes needed to close.
	HalfOpenSuccesses int
	// ShouldTrip determines whether a response/error should count as a failure.
	ShouldTrip func(*http.Response, error) bool
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxFailures:       5,
		ResetTimeout:      30 * time.Second,
		HalfOpenSuccesses: 2,
		ShouldTrip: func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp != nil && resp.StatusCode >= 500
		},
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu  sync.RWMutex
	cfg Config

	state           State
	failures        int
	successes       int
	lastFailureTime time.Time
}

// New creates a new CircuitBreaker with the given config.
func New(cfg Config) *CircuitBreaker {
	return &CircuitBreaker{cfg: cfg, state: StateClosed}
}

// Allow reports whether a request should be allowed through.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(cb.lastFailureTime) >= cb.cfg.ResetTimeout {
			cb.state = StateHalfOpen
			cb.successes = 0
			return nil
		}
		return ErrOpen
	case StateHalfOpen:
		return nil
	}
	return nil
}

// Record records the outcome of a request.
func (cb *CircuitBreaker) Record(resp *http.Response, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	failed := cb.cfg.ShouldTrip(resp, err)

	switch cb.state {
	case StateClosed:
		if failed {
			cb.failures++
			cb.lastFailureTime = time.Now()
			if cb.failures >= cb.cfg.MaxFailures {
				cb.state = StateOpen
			}
		} else {
			cb.failures = 0
		}

	case StateHalfOpen:
		if failed {
			cb.state = StateOpen
			cb.failures = 0
			cb.lastFailureTime = time.Now()
		} else {
			cb.successes++
			if cb.successes >= cb.cfg.HalfOpenSuccesses {
				cb.state = StateClosed
				cb.failures = 0
				cb.successes = 0
			}
		}
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset manually resets the circuit to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
}
