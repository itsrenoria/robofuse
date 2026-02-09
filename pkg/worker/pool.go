package worker

import (
	"sync"
)

// pool.go provides a bounded worker pool abstraction.

// Pool manages a pool of concurrent workers
type Pool struct {
	maxWorkers int
	sem        chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
}

// NewPool creates a new worker pool with the specified number of workers
func NewPool(maxWorkers int) *Pool {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	return &Pool{
		maxWorkers: maxWorkers,
		sem:        make(chan struct{}, maxWorkers),
	}
}

// Submit submits a job to the worker pool
func (p *Pool) Submit(job func()) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Acquire semaphore
		p.sem <- struct{}{}
		defer func() { <-p.sem }()

		job()
	}()
}

// Wait waits for all jobs to complete
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Result holds the result of a job with an optional error
type Result[T any] struct {
	Value T
	Error error
}

// BatchResult processes items in parallel and returns results
type BatchResult[T any, R any] struct {
	pool    *Pool
	results chan Result[R]
	wg      sync.WaitGroup
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor[T any, R any](maxWorkers int) *BatchResult[T, R] {
	return &BatchResult[T, R]{
		pool:    NewPool(maxWorkers),
		results: make(chan Result[R], 1000),
	}
}

// Process processes items with the given function
func (b *BatchResult[T, R]) Process(items []T, fn func(T) (R, error)) []Result[R] {
	// Submit all jobs
	for _, item := range items {
		item := item // capture for goroutine
		b.wg.Add(1)
		b.pool.Submit(func() {
			defer b.wg.Done()
			result, err := fn(item)
			b.results <- Result[R]{Value: result, Error: err}
		})
	}

	// Wait and close results channel
	go func() {
		b.wg.Wait()
		close(b.results)
	}()

	// Collect results
	var results []Result[R]
	for r := range b.results {
		results = append(results, r)
	}

	return results
}

// ProcessWithProgress processes items and calls progress callback
func ProcessWithProgress[T any, R any](
	items []T,
	maxWorkers int,
	fn func(T) (R, error),
	progress func(completed, total int),
) ([]R, []error) {
	pool := NewPool(maxWorkers)

	var mu sync.Mutex
	var results []R
	var errors []error
	completed := 0
	total := len(items)

	for _, item := range items {
		item := item
		pool.Submit(func() {
			result, err := fn(item)

			mu.Lock()
			if err != nil {
				errors = append(errors, err)
			} else {
				results = append(results, result)
			}
			completed++
			if progress != nil {
				progress(completed, total)
			}
			mu.Unlock()
		})
	}

	pool.Wait()
	return results, errors
}
