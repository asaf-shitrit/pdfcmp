package pool

import (
	"context"
	"runtime"
	"sync"
)

// Job represents a unit of work to be processed
type Job[T any, R any] struct {
	ID    int
	Input T
}

// Result represents the result of processing a job
type Result[R any] struct {
	ID    int
	Value R
	Err   error
}

// WorkerFunc is the function type for processing jobs
type WorkerFunc[T any, R any] func(ctx context.Context, input T) (R, error)

// Pool manages a pool of workers for concurrent job processing
type Pool[T any, R any] struct {
	workers int
	fn      WorkerFunc[T, R]
}

// New creates a new worker pool with the specified number of workers.
// If workers <= 0, it defaults to runtime.NumCPU().
func New[T any, R any](workers int, fn WorkerFunc[T, R]) *Pool[T, R] {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &Pool[T, R]{
		workers: workers,
		fn:      fn,
	}
}

// Process processes all jobs concurrently and returns results in order.
// It respects context cancellation and returns early if the context is cancelled.
func (p *Pool[T, R]) Process(ctx context.Context, jobs []Job[T, R]) ([]Result[R], error) {
	if len(jobs) == 0 {
		return nil, nil
	}

	// Limit workers to number of jobs if fewer jobs than workers
	numWorkers := p.workers
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	jobCh := make(chan indexedJob[T, R], len(jobs))
	results := make([]Result[R], len(jobs))

	// Start workers. Each worker writes only the slots of the jobs it received,
	// and every slot belongs to exactly one job, so no locking is needed.
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(ctx, jobCh, results)
		}()
	}

	// Send jobs, tagged with their slice position so results never depend on
	// the caller-supplied Job.ID being unique.
	go func() {
		defer close(jobCh)
		for pos, job := range jobs {
			select {
			case <-ctx.Done():
				return // Exit goroutine immediately on cancellation
			case jobCh <- indexedJob[T, R]{pos: pos, job: job}:
			}
		}
	}()

	// Wait for workers to finish before reading results.
	wg.Wait()

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return results, nil
}

// indexedJob pairs a job with its position in the input slice. The position,
// not Job.ID, decides where the result is stored, because callers of the
// exported Process may reuse IDs and IDs would then collide.
type indexedJob[T any, R any] struct {
	pos int
	job Job[T, R]
}

// worker processes jobs from the job channel and stores each result at the
// job's input position. Job.ID is copied into the Result unchanged.
func (p *Pool[T, R]) worker(ctx context.Context, jobs <-chan indexedJob[T, R], results []Result[R]) {
	for ij := range jobs {
		select {
		case <-ctx.Done():
			results[ij.pos] = Result[R]{
				ID:  ij.job.ID,
				Err: ctx.Err(),
			}
			return
		default:
		}

		value, err := p.fn(ctx, ij.job.Input)
		results[ij.pos] = Result[R]{
			ID:    ij.job.ID,
			Value: value,
			Err:   err,
		}
	}
}

// ProcessFunc is a convenience function that creates jobs from inputs and processes them.
func ProcessFunc[T any, R any](ctx context.Context, workers int, inputs []T, fn WorkerFunc[T, R]) ([]R, error) {
	pool := New(workers, fn)

	jobs := make([]Job[T, R], len(inputs))
	for i, input := range inputs {
		jobs[i] = Job[T, R]{
			ID:    i,
			Input: input,
		}
	}

	results, err := pool.Process(ctx, jobs)
	if err != nil {
		return nil, err
	}

	values := make([]R, len(results))
	for i, result := range results {
		if result.Err != nil {
			return nil, result.Err
		}
		values[i] = result.Value
	}

	return values, nil
}

// Map applies a function to each element concurrently using a worker pool.
func Map[T any, R any](ctx context.Context, workers int, items []T, fn func(T) (R, error)) ([]R, error) {
	return ProcessFunc(ctx, workers, items, func(ctx context.Context, input T) (R, error) {
		return fn(input)
	})
}

// MapWithIndex applies a function to each element with its index.
func MapWithIndex[T any, R any](ctx context.Context, workers int, items []T, fn func(int, T) (R, error)) ([]R, error) {
	type indexedInput struct {
		index int
		value T
	}

	inputs := make([]indexedInput, len(items))
	for i, item := range items {
		inputs[i] = indexedInput{index: i, value: item}
	}

	return ProcessFunc(ctx, workers, inputs, func(ctx context.Context, input indexedInput) (R, error) {
		return fn(input.index, input.value)
	})
}

// ForEach applies a function to each element concurrently, discarding results.
func ForEach[T any](ctx context.Context, workers int, items []T, fn func(T) error) error {
	_, err := ProcessFunc(ctx, workers, items, func(ctx context.Context, input T) (struct{}, error) {
		return struct{}{}, fn(input)
	})
	return err
}
