package pool

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestPool_Process(t *testing.T) {
	t.Run("Basic processing", func(t *testing.T) {
		inputs := []int{1, 2, 3, 4, 5}
		fn := func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		}
		
		results, err := ProcessFunc(context.Background(), 2, inputs, fn)
		if err != nil {
			t.Fatalf("ProcessFunc failed: %v", err)
		}
		
		if len(results) != len(inputs) {
			t.Errorf("Expected %d results, got %d", len(inputs), len(results))
		}
		
		for i, res := range results {
			if res != inputs[i]*2 {
				t.Errorf("Result %d: expected %d, got %d", i, inputs[i]*2, res)
			}
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		inputs := []int{1, 2, 3}
		fn := func(ctx context.Context, input int) (int, error) {
			if input == 2 {
				return 0, errors.New("error on 2")
			}
			return input, nil
		}
		
		results, err := ProcessFunc(context.Background(), 2, inputs, fn)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if results != nil {
			t.Error("Expected nil results on error")
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		inputs := []int{1, 2, 3, 4, 5}
		ctx, cancel := context.WithCancel(context.Background())
		
		// Use a mutex to safely append to processed
		var mu sync.Mutex
		var processed []int
		
		fn := func(ctx context.Context, input int) (int, error) {
			if input == 3 {
				cancel()
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				mu.Lock()
				processed = append(processed, input)
				mu.Unlock()
				return input, nil
			}
		}
		
		_, err := ProcessFunc(ctx, 2, inputs, fn)
		if err == nil {
			t.Error("Expected context error")
		}
	})
	
	t.Run("Concurrency check", func(t *testing.T) {
		workers := 5
		inputs := make([]int, workers)
		ready := make(chan struct{})
		start := make(chan struct{})
		
		var wg sync.WaitGroup
		wg.Add(workers)
		
		fn := func(ctx context.Context, input int) (int, error) {
			wg.Done()
			<-start // Wait for signal to proceed
			return input, nil
		}
		
		pool := New(workers, fn)
		jobs := make([]Job[int, int], len(inputs))
		for i := range inputs {
			jobs[i] = Job[int, int]{ID: i, Input: i}
		}
		
		go func() {
			wg.Wait() // Wait for all workers to start
			close(ready)
		}()
		
		go pool.Process(context.Background(), jobs)
		
		select {
		case <-ready:
			// All workers started concurrently
			close(start)
		case <-time.After(100 * time.Millisecond):
			t.Error("Timeout waiting for concurrency")
			close(start) // Cleanup
		}
	})
}

func TestHelpers(t *testing.T) {
	t.Run("Map", func(t *testing.T) {
		inputs := []int{1, 2, 3}
		results, err := Map(context.Background(), 2, inputs, func(i int) (int, error) {
			return i * i, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		expected := []int{1, 4, 9}
		for i, res := range results {
			if res != expected[i] {
				t.Errorf("Idx %d: expected %d, got %d", i, expected[i], res)
			}
		}
	})
	
	t.Run("ForEach", func(t *testing.T) {
		inputs := []int{1, 2, 3}
		var mu sync.Mutex
		var processed []int
		
		err := ForEach(context.Background(), 2, inputs, func(i int) error {
			mu.Lock()
			processed = append(processed, i)
			mu.Unlock()
			return nil
		})
		
		if err != nil {
			t.Fatal(err)
		}
		
		sort.Ints(processed)
		if len(processed) != 3 {
			t.Errorf("Expected 3 items processed, got %d", len(processed))
		}
	})
}
