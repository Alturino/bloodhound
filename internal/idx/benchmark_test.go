package idx

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Job represents work to be done by a worker
type Job struct {
	ID    int
	Delay time.Duration
}

// Result holds benchmark results
type Result struct {
	Workers    int
	Duration   time.Duration
	PeakGo     int32
	TotalJobs  int
}

// simulateSequential simulates the original sequential approach
func simulateSequential(ctx context.Context, jobs []Job) time.Duration {
	start := time.Now()
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return time.Since(start)
		case <-time.After(job.Delay):
		}
	}
	return time.Since(start)
}

// simulateWorkerPool simulates the worker pool approach
func simulateWorkerPool(ctx context.Context, jobs []Job, workerCount int) time.Duration {
	start := time.Now()
	jobCh := make(chan Job, len(jobs))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				select {
				case <-ctx.Done():
					return
				case <-time.After(job.Delay):
				}
			}
		}()
	}

	for _, job := range jobs {
		select {
		case jobCh <- job:
		case <-ctx.Done():
			break
		}
	}
	close(jobCh)
	wg.Wait()
	return time.Since(start)
}

// simulatePipeline simulates the pipeline approach
func simulatePipeline(ctx context.Context, jobs []Job, workerCount int) time.Duration {
	start := time.Now()

	// First pass: collect all work
	var collected []Job
	for _, job := range jobs {
		collected = append(collected, job)
	}

	if len(collected) == 0 {
		return time.Since(start)
	}

	// Second pass: fan-out to workers
	jobCh := make(chan Job, len(collected))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				select {
				case <-ctx.Done():
					return
				case <-time.After(job.Delay):
				}
			}
		}()
	}

	for _, job := range collected {
		select {
		case jobCh <- job:
		case <-ctx.Done():
			break
		}
	}
	close(jobCh)
	wg.Wait()
	return time.Since(start)
}

// BenchmarkComparison compares all three approaches
func TestBenchmarkComparison(t *testing.T) {
	// Configuration
	pageCount := 10
	fetchLatency := 10 * time.Millisecond
	insertLatency := 5 * time.Millisecond

	ctx := context.Background()
	totalJobs := pageCount

	// Create jobs simulating page fetch + process
	jobs := make([]Job, totalJobs)
	for i := range jobs {
		// Each job simulates fetch (10ms) + process (5ms) = 15ms per page
		jobs[i] = Job{ID: i, Delay: fetchLatency + insertLatency}
	}

	workerCounts := []int{1, 3, 5, 10}

	t.Log("\n=== Benchmark Results ===")
	t.Logf("%-12s | %-12s | %-12s | %-12s | %-12s", "Workers", "Sequential", "WorkerPool", "Pipeline", "Speedup")
	t.Log("---------------------------------------------------------------------------")

	for _, wc := range workerCounts {
		// Sequential (original approach - 1 worker processing sequentially)
		seqDuration := simulateSequential(ctx, jobs)

		// Worker Pool
		wpDuration := simulateWorkerPool(ctx, jobs, wc)

		// Pipeline
		plDuration := simulatePipeline(ctx, jobs, wc)

		speedup := float64(seqDuration) / float64(wpDuration)

		t.Logf("%-12d | %-12v | %-12v | %-12v | %.2fx",
			wc, seqDuration, wpDuration, plDuration, speedup)
	}
}

// BenchmarkGoroutineCount measures peak goroutines for each approach
func TestGoroutineCount(t *testing.T) {
	// Configuration
	pageCount := 10
	fetchLatency := 10 * time.Millisecond
	insertLatency := 5 * time.Millisecond

	ctx := context.Background()
	totalJobs := pageCount

	jobs := make([]Job, totalJobs)
	for i := range jobs {
		jobs[i] = Job{ID: i, Delay: fetchLatency + insertLatency}
	}

	workerCounts := []int{1, 3, 5, 10}

	t.Log("\n=== Goroutine Count ===")
	t.Logf("%-12s | %-15s | %-15s", "Workers", "WorkerPool", "Pipeline")
	t.Log("---------------------------------------------------")

	for _, wc := range workerCounts {
		// Worker Pool goroutine count
		stop := make(chan struct{})
		goroutineCh := make(chan int32, 1)

		go func() {
			var max int32
			ticker := time.NewTicker(100 * time.Microsecond)
			for {
				select {
				case <-stop:
					ticker.Stop()
					goroutineCh <- max
					return
				case <-ticker.C:
					n := int32(runtime.NumGoroutine())
					if n > max {
						max = n
					}
				}
			}
		}()

		simulateWorkerPool(ctx, jobs, wc)
		close(stop)
		wpPeak := <-goroutineCh

		// Pipeline goroutine count
		stop2 := make(chan struct{})
		goroutineCh2 := make(chan int32, 1)

		go func() {
			var max int32
			ticker := time.NewTicker(100 * time.Microsecond)
			for {
				select {
				case <-stop2:
					ticker.Stop()
					goroutineCh2 <- max
					return
				case <-ticker.C:
					n := int32(runtime.NumGoroutine())
					if n > max {
						max = n
					}
				}
			}
		}()

		simulatePipeline(ctx, jobs, wc)
		close(stop2)
		plPeak := <-goroutineCh2

		t.Logf("%-12d | %-15d | %-15d", wc, wpPeak, plPeak)
	}
}

// BenchmarkMemoryUsage estimates memory allocation
func TestMemoryUsage(t *testing.T) {
	// Memory estimation
	// Each announcement ~1KB, each page 50 announcements, 10 pages
	annPerPage := 50
	pages := 10
	bytesPerAnn := 1024

	sequentialMem := int64(annPerPage * pages * bytesPerAnn)
	workerPoolMem := sequentialMem // Same data, just distributed
	pipelineMem := sequentialMem * 2 // Stores all pages before processing

	t.Log("\n=== Memory Usage ===")
	t.Logf("%-15s | %-15s | %-15s", "Type", "Memory (MB)", "Notes")
	t.Log("-------------------------------------------------------------------")
	t.Logf("%-15s | %-15.2f | %-15s", "Sequential", float64(sequentialMem)/1024/1024, "Single pointer")
	t.Logf("%-15s | %-15.2f | %-15s", "WorkerPool", float64(workerPoolMem)/1024/1024, "Distributed")
	t.Logf("%-15s | %-15.2f | %-15s", "Pipeline", float64(pipelineMem)/1024/1024, "All pages buffered")
}

// RunRaceDetector runs race detector on the concurrency patterns
func TestRaceDetector(t *testing.T) {
	fetchLatency := 1 * time.Millisecond
	insertLatency := 1 * time.Millisecond

	ctx := context.Background()
	jobs := make([]Job, 10)
	for i := range jobs {
		jobs[i] = Job{ID: i, Delay: fetchLatency + insertLatency}
	}

	workerCounts := []int{1, 3, 5}

	for _, wc := range workerCounts {
		t.Run(fmt.Sprintf("workerpool-w%d", wc), func(t *testing.T) {
			for i := 0; i < 5; i++ {
				simulateWorkerPool(ctx, jobs, wc)
			}
		})

		t.Run(fmt.Sprintf("pipeline-w%d", wc), func(t *testing.T) {
			for i := 0; i < 5; i++ {
				simulatePipeline(ctx, jobs, wc)
			}
		})
	}
}