package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
					Str("filename", job.Attachment.OriginalFilename).
					Str("url", job.Attachment.FullSavePath).
					Logger()
				if !strings.HasSuffix(job.Attachment.OriginalFilename, ".pdf") {
					err := errors.New("attachment is not a pdf, skipping")
					jobLogger.Debug().Err(err).Msg(err.Error())
					resCh <- jobs.DownloadRes{Err: err, URL: job.Attachment.FullSavePath, AttachmentID: job.Attachment.ID, WorkerID: workerID, JobID: job.JobID, Emiten: job.Emiten}
					continue
				}
				jobLogger.Debug().Msg("downloading file")
				ctx = jobLogger.WithContext(ctx)
				err := track.repo.DownloadFile(ctx, job.Emiten, job.Attachment, job.TglPengumuman)
				if err != nil {
					err = fmt.Errorf("failed to download file with error: %w", err)
					jobLogger.Error().Err(err).Msg(err.Error())
					resCh <- jobs.DownloadRes{Err: err, URL: job.Attachment.FullSavePath, AttachmentID: job.Attachment.ID, WorkerID: workerID, JobID: job.JobID, Emiten: job.Emiten}
					continue
				}
				resCh <- jobs.DownloadRes{Err: err, URL: job.Attachment.FullSavePath, AttachmentID: job.Attachment.ID, WorkerID: workerID, JobID: job.JobID, Emiten: job.Emiten}
				jobLogger.Info().Msg("successfully downloaded file")
				time.Sleep(time.Second * 7)
			}
		}
	}
}
