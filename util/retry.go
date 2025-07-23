package util

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries      int
	BaseDelay       time.Duration
	MaxDelay        time.Duration
	ShouldRetryFunc func(error) bool
}

// DefaultRetryConfig provides sensible defaults for retry operations
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      5,
		BaseDelay:       10 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		ShouldRetryFunc: nil, // No retry by default
	}
}


// Retry implements exponential backoff retry logic with configurable error matching
func Retry(ctx context.Context, config RetryConfig, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(config.BaseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}

			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.1)
			delay = delay + jitter

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if this error should be retried using the configured function
		if config.ShouldRetryFunc != nil && config.ShouldRetryFunc(err) {
			continue
		}

		// Error doesn't match retry criteria, don't retry
		return err
	}

	return fmt.Errorf("operation failed after %d retries, last error: %w", config.MaxRetries, lastErr)
}