package worker

import (
	"context"
	"log"
)

type WorkerFunc[I any, O any] func(ctx context.Context, workerID int, jobCh <-chan I, resCh chan<- O)

func WorkerPool[I any, O any](
	ctx context.Context,
	pool int,
	jobCh <-chan I,
	resCh chan<- O,
	worker WorkerFunc[I, O],
) {
	select {
	case <-ctx.Done():
		log.Println("context done, stopping worker pool")
		return
	default:
		for i := range pool {
			go worker(ctx, i, jobCh, resCh)
		}
	}
}
