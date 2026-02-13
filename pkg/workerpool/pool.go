// Package workerpool provides a bounded worker pool for concurrent
// execution of Windows commands from WSL. It limits the number of
// simultaneous process spawns to avoid overwhelming the system.
package workerpool

import (
	"context"
	"runtime"
	"sync"

	"github.com/sibikrish3000/gowinbridge/pkg/bridge"
)

// Result wraps the output of a command execution along with the
// original config that produced it.
type Result struct {
	Config bridge.CommandConfig
	Output bridge.Output
	Err    error
}

// ExecutorFunc is the function signature used to execute a command.
// This abstraction allows injecting a mock executor for testing.
type ExecutorFunc func(ctx context.Context, config bridge.CommandConfig) (bridge.Output, error)

// Pool manages a bounded set of workers that process CommandConfig jobs.
type Pool struct {
	concurrency int
	executor    ExecutorFunc
	jobs        chan bridge.CommandConfig
	results     chan Result
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	startOnce   sync.Once
}

// NewPool creates a worker pool with the given concurrency limit.
// If concurrency <= 0, it defaults to runtime.NumCPU().
// The executor function is used to process each job.
func NewPool(concurrency int, executor ExecutorFunc) *Pool {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pool{
		concurrency: concurrency,
		executor:    executor,
		jobs:        make(chan bridge.CommandConfig, concurrency*2),
		results:     make(chan Result, concurrency*2),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// start launches the worker goroutines (called once).
func (p *Pool) start() {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	// Close results channel when all workers finish.
	go func() {
		p.wg.Wait()
		close(p.results)
	}()
}

// worker pulls jobs from the channel and executes them.
func (p *Pool) worker() {
	defer p.wg.Done()
	for config := range p.jobs {
		select {
		case <-p.ctx.Done():
			p.results <- Result{
				Config: config,
				Err:    p.ctx.Err(),
			}
		default:
			output, err := p.executor(p.ctx, config)
			p.results <- Result{
				Config: config,
				Output: output,
				Err:    err,
			}
		}
	}
}

// Submit adds a command to the work queue. It starts workers on first call.
// Blocks if the job buffer is full.
func (p *Pool) Submit(config bridge.CommandConfig) {
	p.startOnce.Do(p.start)
	p.jobs <- config
}

// Results returns the channel from which completed results can be read.
// The channel is closed after Shutdown completes.
func (p *Pool) Results() <-chan Result {
	p.startOnce.Do(p.start)
	return p.results
}

// Shutdown signals that no more jobs will be submitted.
// It closes the job channel, waits for in-flight work to finish,
// and then the results channel is closed automatically.
func (p *Pool) Shutdown() {
	close(p.jobs)
	p.wg.Wait()
}

// Cancel terminates the pool context, causing workers to abort pending jobs.
func (p *Pool) Cancel() {
	p.cancel()
}
