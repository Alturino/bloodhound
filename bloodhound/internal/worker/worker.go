package worker

import (
	"context"

	"github.com/rs/zerolog"
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
	logger := zerolog.Ctx(ctx).With().Logger()
	select {
	case <-ctx.Done():
		logger.Info().Msg("context done, stopping worker pool")
		return
	default:
		for i := range pool {
			go workerFunc(ctx, i, jobCh, resCh, stopCh)
		}
	}
}
