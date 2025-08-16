package worker

import (
	"context"
	"log"
)

type WorkerFunc[I any, O any] func(ctx context.Context, workerID int, jobCh <-chan I, resCh chan<- O, stopCh <-chan struct{})

func WorkerPool[I any, O any](
	ctx context.Context,
	pool int,
	jobCh <-chan I,
	resCh chan<- O,
	stopCh <-chan struct{},
	workerFunc WorkerFunc[I, O],
) {
	select {
	case <-ctx.Done():
		log.Println("context done, stopping worker pool")
		return
	default:
		for i := range pool {
			go workerFunc(ctx, i, jobCh, resCh, stopCh)
		}
	}
}
