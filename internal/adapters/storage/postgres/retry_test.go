package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryWithBackoff_FirstAttemptSucceeds(t *testing.T) {
	calls := 0
	sleep := func(time.Duration) {}
	err := retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        sleep,
	}, func(ctx context.Context) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryWithBackoff_EventualSuccess(t *testing.T) {
	calls := 0
	sleeps := []time.Duration{}
	sleep := func(d time.Duration) { sleeps = append(sleeps, d) }
	target := errors.New("transient")
	err := retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        sleep,
	}, func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return target
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, sleeps)
}

func TestRetryWithBackoff_Exhausted(t *testing.T) {
	target := errors.New("always fails")
	err := retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        func(time.Duration) {},
	}, func(ctx context.Context) error {
		return target
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, target)
}

func TestRetryWithBackoff_CapsDelayAtMax(t *testing.T) {
	sleeps := []time.Duration{}
	_ = retryWithBackoff(context.Background(), retryConfig{
		MaxAttempts:  6,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     300 * time.Millisecond,
		Sleep:        func(d time.Duration) { sleeps = append(sleeps, d) },
	}, func(ctx context.Context) error {
		return errors.New("nope")
	})
	// attempts 1→2: 100ms, 2→3: 200ms, 3→4: 300ms (capped), 4→5: 300ms, 5→6: 300ms
	assert.Equal(t, []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		300 * time.Millisecond,
		300 * time.Millisecond,
	}, sleeps)
}

func TestRetryWithBackoff_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := retryWithBackoff(ctx, retryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        func(time.Duration) {},
	}, func(ctx context.Context) error {
		calls++
		return errors.New("x")
	})
	require.Error(t, err)
	assert.LessOrEqual(t, calls, 1)
}
