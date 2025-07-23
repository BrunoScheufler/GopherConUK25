package util

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	tests := []struct {
		name          string
		config        RetryConfig
		errorSequence []error
		expectedError string
		expectSuccess bool
		maxDuration   time.Duration
	}{
		{
			name: "success on first attempt",
			config: RetryConfig{
				MaxRetries: 3,
				BaseDelay:  10 * time.Millisecond,
				MaxDelay:   100 * time.Millisecond,
			},
			errorSequence: []error{nil},
			expectSuccess: true,
			maxDuration:   50 * time.Millisecond,
		},
		{
			name: "success after retries with retriable error",
			config: RetryConfig{
				MaxRetries:      3,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			},
			errorSequence: []error{
				errors.New("temporary error 1"),
				errors.New("temporary error 2"),
				nil,
			},
			expectSuccess: true,
			maxDuration:   200 * time.Millisecond,
		},
		{
			name: "non-retriable error fails immediately",
			config: RetryConfig{
				MaxRetries:      3,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			},
			errorSequence: []error{
				errors.New("syntax error"),
			},
			expectedError: "syntax error",
			expectSuccess: false,
			maxDuration:   50 * time.Millisecond,
		},
		{
			name: "exceeds max retries with 0 retries",
			config: RetryConfig{
				MaxRetries:      0,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			},
			errorSequence: []error{
				errors.New("temporary error"),
			},
			expectedError: "operation failed after 0 retries",
			expectSuccess: false,
			maxDuration:   50 * time.Millisecond,
		},
		{
			name: "exceeds max retries with 3 retries",
			config: RetryConfig{
				MaxRetries:      3,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			},
			errorSequence: []error{
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
			},
			expectedError: "operation failed after 3 retries",
			expectSuccess: false,
			maxDuration:   500 * time.Millisecond,
		},
		{
			name: "context cancellation",
			config: RetryConfig{
				MaxRetries:      10,
				BaseDelay:       50 * time.Millisecond,
				MaxDelay:        200 * time.Millisecond,
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			},
			errorSequence: []error{
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
			},
			expectedError: "context",
			expectSuccess: false,
			maxDuration:   150 * time.Millisecond, // Cancel context before retries complete
		},
		{
			name: "exponential backoff capped at max delay",
			config: RetryConfig{
				MaxRetries:      5,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        50 * time.Millisecond, // Low max to test capping
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			},
			errorSequence: []error{
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
				errors.New("temporary error"),
			},
			expectedError: "operation failed after 5 retries",
			expectSuccess: false,
			maxDuration:   1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			start := time.Now()

			// Create context with timeout for cancellation test
			ctx := context.Background()
			var cancel context.CancelFunc
			if strings.Contains(tt.name, "context cancellation") {
				ctx, cancel = context.WithTimeout(ctx, tt.maxDuration)
				defer cancel()
			}

			err := Retry(ctx, tt.config, func() error {
				if callCount < len(tt.errorSequence) {
					err := tt.errorSequence[callCount]
					callCount++
					return err
				}
				// If we've exhausted the error sequence, return success
				return nil
			})

			duration := time.Since(start)

			// Check success/failure
			if tt.expectSuccess {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error, got success")
				}
				if tt.expectedError != "" && !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			}

			// Check duration is reasonable (allow 20% tolerance for timing variations)
			tolerance := time.Duration(float64(tt.maxDuration) * 1.2)
			if duration > tolerance {
				t.Errorf("operation took too long: %v > %v (with tolerance)", duration, tolerance)
			}

			// Check that function was called the expected number of times
			expectedCalls := len(tt.errorSequence)
			if tt.expectSuccess {
				// For success cases, we might call fewer times than the error sequence
				if callCount > expectedCalls {
					t.Errorf("expected at most %d calls, got %d", expectedCalls, callCount)
				}
			} else if !strings.Contains(tt.name, "context cancellation") && !strings.Contains(tt.name, "non-retriable") {
				// For failure cases (except cancellation and non-BUSY), we should exhaust retries
				expectedMinCalls := tt.config.MaxRetries + 1
				if callCount < expectedMinCalls {
					t.Errorf("expected at least %d calls, got %d", expectedMinCalls, callCount)
				}
			}
		})
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	expectedMaxRetries := 5
	expectedBaseDelay := 10 * time.Millisecond
	expectedMaxDelay := 1 * time.Second

	if config.MaxRetries != expectedMaxRetries {
		t.Errorf("expected MaxRetries %d, got %d", expectedMaxRetries, config.MaxRetries)
	}

	if config.BaseDelay != expectedBaseDelay {
		t.Errorf("expected BaseDelay %v, got %v", expectedBaseDelay, config.BaseDelay)
	}

	if config.MaxDelay != expectedMaxDelay {
		t.Errorf("expected MaxDelay %v, got %v", expectedMaxDelay, config.MaxDelay)
	}
}

func TestRetry_ErrorDetection(t *testing.T) {
	tests := []struct {
		name        string
		error       error
		shouldRetry bool
	}{
		{
			name:        "temporary error - should retry",
			error:       errors.New("temporary connection error"),
			shouldRetry: true,
		},
		{
			name:        "another temporary error - should retry",
			error:       errors.New("temporary service unavailable"),
			shouldRetry: true,
		},
		{
			name:        "permanent error - should not retry",
			error:       errors.New("syntax error near 'SELECT'"),
			shouldRetry: false,
		},
		{
			name:        "constraint violation - should not retry",
			error:       errors.New("UNIQUE constraint failed"),
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			config := RetryConfig{
				MaxRetries:      2,
				BaseDelay:       1 * time.Millisecond,
				MaxDelay:        10 * time.Millisecond,
				ShouldRetryFunc: func(err error) bool { return strings.Contains(err.Error(), "temporary") },
			}

			err := Retry(context.Background(), config, func() error {
				callCount++
				return tt.error
			})

			if tt.shouldRetry {
				// Should attempt multiple times for retriable errors
				expectedCalls := config.MaxRetries + 1
				if callCount != expectedCalls {
					t.Errorf("expected %d calls for retriable error, got %d", expectedCalls, callCount)
				}
			} else {
				// Should fail immediately for non-retriable errors
				if callCount != 1 {
					t.Errorf("expected 1 call for non-retriable error, got %d", callCount)
				}
			}

			// Should always return an error in these test cases
			if err == nil {
				t.Errorf("expected error, got success")
			}
		})
	}
}