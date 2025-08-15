package internal

import (
	"context"
	"fmt"

	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/response"
	"github.com/Alturino/bloodhound/internal/worker"
)

type Track struct {
	repository repository.Repository
}

func NewTrack(repository repository.Repository) *Track {
	return &Track{repository: repository}
}

func (t Track) TrackTillEmpty(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) {
	pool := 10
	jobCh := make(chan jobs.FetchJob, pool)
	defer close(jobCh)

	resCh := make(chan response.Response, pool)
	defer close(resCh)

	stopCh := make(chan struct{}, pool)
	defer close(stopCh)

	go worker.WorkerPool(
		ctx,
		pool,
		jobCh,
		resCh,
		func(ctx context.Context, workerID int, jobCh <-chan jobs.FetchJob, resCh chan<- response.Response) {
		},
	)
}

func (t Track) Track(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) (response.Response, error) {
	res, err := t.repository.Get(ctx, emiten, keyword, page, pageSize)
	if err != nil {
		err = fmt.Errorf("Track failed with error: %w", err)
		return response.Response{}, err
	}
	return res, nil
}
