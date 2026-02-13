package workerpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sibikrish3000/gowinbridge/pkg/bridge"
)

// mockExecutor returns a simple executor that records calls and returns a fixed output.
func mockExecutor(delay time.Duration) (ExecutorFunc, *atomic.Int32) {
	var count atomic.Int32
	fn := func(ctx context.Context, config bridge.CommandConfig) (bridge.Output, error) {
		count.Add(1)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return bridge.Output{}, ctx.Err()
		}
		return bridge.Output{
			Stdout:   "ok: " + config.Command,
			ExitCode: 0,
		}, nil
	}
	return fn, &count
}

func TestPoolBasicExecution(t *testing.T) {
	executor, _ := mockExecutor(10 * time.Millisecond)
	pool := NewPool(2, executor)

	pool.Submit(bridge.CommandConfig{Command: "cmd1.exe"})
	pool.Submit(bridge.CommandConfig{Command: "cmd2.exe"})
	pool.Submit(bridge.CommandConfig{Command: "cmd3.exe"})

	// Close jobs channel so workers can exit after processing.
	go func() {
		// Give time for submits to be queued.
		time.Sleep(50 * time.Millisecond)
		pool.Shutdown()
	}()

	results := make([]Result, 0)
	for r := range pool.Results() {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error for %s: %v", r.Config.Command, r.Err)
		}
	}
}

func TestPoolConcurrencyLimit(t *testing.T) {
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	executor := func(ctx context.Context, config bridge.CommandConfig) (bridge.Output, error) {
		cur := current.Add(1)

		// Track max concurrent via CAS loop.
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}

		time.Sleep(50 * time.Millisecond)
		current.Add(-1)
		return bridge.Output{Stdout: "done"}, nil
	}

	concurrency := 2
	pool := NewPool(concurrency, executor)

	// Submit more jobs than concurrency.
	for i := 0; i < 6; i++ {
		pool.Submit(bridge.CommandConfig{Command: fmt.Sprintf("job%d.exe", i)})
	}

	// Shut down in background while we drain results.
	go pool.Shutdown()

	count := 0
	for range pool.Results() {
		count++
	}

	if count != 6 {
		t.Fatalf("expected 6 results, got %d", count)
	}

	if mc := maxConcurrent.Load(); mc > int32(concurrency) {
		t.Errorf("max concurrent = %d, exceeds limit of %d", mc, concurrency)
	}
}

func TestPoolDefaultConcurrency(t *testing.T) {
	executor, _ := mockExecutor(1 * time.Millisecond)
	pool := NewPool(0, executor)

	if pool.concurrency <= 0 {
		t.Errorf("default concurrency should be > 0, got %d", pool.concurrency)
	}

	pool.Submit(bridge.CommandConfig{Command: "test.exe"})
	go pool.Shutdown()

	for range pool.Results() {
	}
}

func TestPoolWithErrors(t *testing.T) {
	errExecutor := func(ctx context.Context, config bridge.CommandConfig) (bridge.Output, error) {
		return bridge.Output{}, fmt.Errorf("simulated failure")
	}

	pool := NewPool(1, errExecutor)
	pool.Submit(bridge.CommandConfig{Command: "failing.exe"})
	go pool.Shutdown()

	for r := range pool.Results() {
		if r.Err == nil {
			t.Error("expected error, got nil")
		}
	}
}

func TestPoolCancel(t *testing.T) {
	// Use a slow executor so jobs are still pending when we cancel.
	executor, _ := mockExecutor(5 * time.Second)
	pool := NewPool(1, executor)

	pool.Submit(bridge.CommandConfig{Command: "slow1.exe"})
	pool.Submit(bridge.CommandConfig{Command: "slow2.exe"})

	// Cancel after a brief moment, then shutdown.
	go func() {
		time.Sleep(100 * time.Millisecond)
		pool.Cancel()
		pool.Shutdown()
	}()

	// Drain results concurrently — critical to avoid deadlock.
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for {
		select {
		case _, ok := <-pool.Results():
			if !ok {
				return // Channel closed, we're done.
			}
		case <-timer.C:
			t.Fatal("test timed out waiting for results")
			return
		}
	}
}
