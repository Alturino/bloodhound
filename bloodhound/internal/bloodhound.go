package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/response"
	"github.com/Alturino/bloodhound/internal/worker"
)

type Track struct {
	repo             repository.HTTPRepository
	pool             int
	downloadJobCh    chan jobs.DownloadJob
	resDownloadJobCh chan jobs.DownloadRes
	stopDownloadCh   chan struct{}
}

func NewTrack(ctx context.Context, repository repository.HTTPRepository, pool int) Track {
	downloadJobCh := make(chan jobs.DownloadJob, pool)
	resDownloadJobCh := make(chan jobs.DownloadRes, pool)
	stopDownloadCh := make(chan struct{}, pool)
	track := Track{
		repo:             repository,
		pool:             pool,
		downloadJobCh:    downloadJobCh,
		resDownloadJobCh: resDownloadJobCh,
		stopDownloadCh:   stopDownloadCh,
	}

	go worker.WorkerPool(
		ctx,
		pool,
		track.downloadJobCh,
		track.resDownloadJobCh,
		track.stopDownloadCh,
		func(ctx context.Context, workerID int, jobCh <-chan jobs.DownloadJob, resCh chan<- jobs.DownloadRes, stopCh <-chan struct{}) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			logger := zerolog.Ctx(ctx).With().
				Int("worker_id", workerID).
				Logger()
			logger.Debug().Msg("worker started")

			defer close(resCh)
			for {
				select {
				case <-stopCh:
					logger.Info().Msg("received stop signal, stopping worker")
					return
				case <-ctx.Done():
					logger.Info().Msg("received context done, stopping worker")
					return
				case job, ok := <-jobCh:
					if !ok {
						logger.Info().Msg("channel is closed stop receiving from channel")
						return
					}
					logger = logger.With().
						Str("emiten", job.Emiten).
						Str("job_id", job.JobID).
						Logger()
					var wg sync.WaitGroup
					for _, attachment := range job.Attachments {
						logger = logger.With().Str("filename", attachment.OriginalFilename).Logger()
						if !strings.HasSuffix(attachment.OriginalFilename, ".pdf") {
							logger.Debug().Msg("attachment is not a pdf, skipping")
							continue
						}
						wg.Add(1)
						go func(wg *sync.WaitGroup) {
							defer wg.Done()
							logger.Debug().Msg("downloading file")
							err := track.repo.DownloadFile(ctx, job.Emiten, attachment)
							if err != nil {
								err = fmt.Errorf("failed to download file with error: %w", err)
								logger.Error().Err(err).Msg(err.Error())
								return
							}
							logger.Info().Msg("successfully downloaded file")
						}(&wg)
					}
					wg.Wait()
				}
			}
		},
	)
	return track
}

func (t Track) TrackTillEmpty(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := zerolog.Ctx(ctx).
		With().
		Str(logging.KEY_TAG, "Track TrackTillEmpty").
		Str("search_emiten", emiten).
		Str("keyword", keyword).
		Int("starting_page", page).
		Int("pageSize", pageSize).
		Logger()

	currentPage := page
	responses := make([]response.IdxResponse, 0, 100)
	for {
		logger = logger.With().Int("page", currentPage).Logger()
		res, err := t.repo.Get(ctx, emiten, keyword, currentPage, pageSize)
		currentPage++
		if err != nil {
			err = fmt.Errorf("failed to get announcement with error: %w", err)
			logger.Error().Err(err).Msg(err.Error())
			continue
		}
		if len(res.Replies) == 0 {
			err = errors.New("replies is empty, stopping")
			logger.Error().Err(err).Msg(err.Error())
			break
		}
		for _, reply := range res.Replies {
			for _, attachment := range reply.Attachments {
				dLog := logger.With().
					Str("filename", attachment.OriginalFilename).
					Str("url", attachment.FullSavePath).
					Logger()
				err = t.repo.DownloadFile(ctx, emiten, attachment)
				if err != nil {
					dLog.Error().Err(err).Msg(err.Error())
					continue
				}
			}
		}
		// for _, reply := range res.Replies {
		// 	jobID := uuid.NewString()
		// 	logger = logger.With().
		// 		Str("job_id", jobID).
		// 		Str("job_emiten", emiten).
		// 		Logger()
		// 	logger.Debug().Msg("sending job")
		// 	t.downloadJobCh <- jobs.DownloadJob{JobID: jobID, Emiten: emiten, Attachments: reply.Attachments}
		// 	logger.Info().Msg("job sent")
		// }
		responses = append(responses, res)
		log.Println("successfully appending to responses")
	}

	logger.Debug().Msg("creating directory")
	dir := path.Join(common.BloodhoundDir, emiten, "responses")
	if err := os.MkdirAll(dir, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("failed to create directory with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("directory created")

	timestamp := time.Now().Format("2006-01-02_15:04:05")
	filename := fmt.Sprintf("%s_responses.json", timestamp)
	filePath := filepath.Join(dir, filename)
	logger.Debug().Str("filepath", filePath).Msg("creating file")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(0o755))
	if err != nil {
		err = fmt.Errorf("failed to create file with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("file created")

	logger.Debug().Msg("writing responses to file")
	if err = json.NewEncoder(file).Encode(responses); err != nil {
		err = fmt.Errorf("failed to encode json with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("responses written to file")

	logger.Info().Msg("successfully write responses to file")

	return nil
}

func (t Track) Track(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) (response.IdxResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	logger := zerolog.Ctx(ctx).
		With().
		Str("emiten", emiten).
		Str("keyword", keyword).
		Int("page", page).
		Int("pageSize", pageSize).
		Logger()

	logger.Debug().Msg("sending request")
	res, err := t.repo.Get(ctx, emiten, keyword, page, pageSize)
	if err != nil {
		err = fmt.Errorf("Track failed with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return response.IdxResponse{}, err
	}
	logger.Info().Msg("successfully sent request")

	dir := path.Join(common.BloodhoundDir, emiten, "responses")
	timestamp := time.Now().Format("2006-01-02_15:04:05")

	filename := fmt.Sprintf("%s_responses.json", timestamp)
	filePath := filepath.Join(dir, filename)

	logger.Debug().Str("filepath", filePath).Msg("creating file")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(0o755))
	if err != nil {
		err = fmt.Errorf("failed to create file with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return res, nil
	}
	logger.Debug().Msg("file created")

	logger.Debug().Msg("writing responses to file")
	if err := json.NewEncoder(file).Encode(res); err != nil {
		err = fmt.Errorf("failed to write json to file err: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return res, nil
	}
	logger.Debug().Msg("responses written to file")

	return res, nil
}
