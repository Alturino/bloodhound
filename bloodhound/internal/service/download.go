package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/worker"
)

func downloadWorkerFunc() worker.WorkerFunc[jobs.DownloadJob, jobs.DownloadRes] {
	return func(ctx context.Context, workerID int, jobCh <-chan jobs.DownloadJob, resCh chan<- jobs.DownloadRes, stopCh <-chan struct{}) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		logger := zerolog.Ctx(ctx).
			With().
			Str(logging.KEY_TAG, "downloadWorker").
			Int("worker_id", workerID).
			Logger()

		logger.Debug().Msg("worker started")

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
				jobLogger := logger.With().
					Str("emiten", job.Emiten).
					Str("job_id", job.JobID).
					Logger()
				for _, attachment := range job.Attachments {
					attachmentLogger := jobLogger.With().
						Str("filename", attachment.OriginalFilename).
						Str("url", attachment.FullSavePath).
						Logger()
					if !strings.HasSuffix(attachment.OriginalFilename, ".pdf") {
						err := errors.New("attachment is not a pdf, skipping")
						attachmentLogger.Debug().Err(err).Msg(err.Error())
						resCh <- jobs.DownloadRes{Err: err, URL: attachment.FullSavePath, AttachmentID: attachment.ID, WorkerID: workerID}
						continue
					}
					attachmentLogger.Debug().Msg("downloading file")
					ctx = attachmentLogger.WithContext(ctx)
					err := track.repo.DownloadFile(ctx, job.Emiten, attachment)
					if err != nil {
						err = fmt.Errorf("failed to download file with error: %w", err)
						attachmentLogger.Error().Err(err).Msg(err.Error())
						resCh <- jobs.DownloadRes{Err: err, URL: attachment.FullSavePath, AttachmentID: attachment.ID, WorkerID: workerID}
						continue
					}
					resCh <- jobs.DownloadRes{Err: err, URL: attachment.FullSavePath, AttachmentID: attachment.ID, WorkerID: workerID}
					attachmentLogger.Info().Msg("successfully downloaded file")
				}
			}
		}
	}
}
