package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/worker"
)

func downloadWorkerFunc() worker.WorkerFunc[jobs.DownloadAttachmentArgs, jobs.DownloadRes] {
	return func(ctx context.Context, workerID int, jobCh <-chan jobs.DownloadAttachmentArgs, resCh chan<- jobs.DownloadRes, stopCh <-chan struct{}) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		logger := zerolog.Ctx(ctx).
			With().
			Str(constants.KEY_TAG, "downloadWorker").
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
					Str("emiten", job.Announcement.Ticker).
					Str("job_id", job.JobID).
					Str("filename", job.Attachment.Filename).
					Str("url", job.Attachment.DownloadURL).
					Logger()
				if !strings.HasSuffix(job.Attachment.Filename, ".pdf") {
					err := errors.New("attachment is not a pdf, skipping")
					jobLogger.Debug().Err(err).Msg(err.Error())
					resCh <- jobs.DownloadRes{Err: err, URL: job.Attachment.DownloadURL, AttachmentID: job.Attachment.ID, WorkerID: workerID, JobID: job.JobID, Emiten: job.Announcement.Ticker}
					continue
				}
				jobLogger.Debug().Msg("downloading file")
				ctx = jobLogger.WithContext(ctx)
				_, err := track.repo.DownloadFile(
					ctx,
					jobs.DownloadAttachmentArgs{
						JobID:        uuid.NewString(),
						Announcement: job.Announcement,
						Attachment:   job.Attachment,
					},
				)
				if err != nil {
					err = fmt.Errorf("failed to download file with error: %w", err)
					jobLogger.Error().Err(err).Msg(err.Error())
					resCh <- jobs.DownloadRes{Err: err, URL: job.Attachment.DownloadURL, AttachmentID: job.Attachment.ID, WorkerID: workerID, JobID: job.JobID, Emiten: job.Announcement.Ticker}
					continue
				}
				resCh <- jobs.DownloadRes{Err: err, URL: job.Attachment.DownloadURL, AttachmentID: job.Attachment.ID, WorkerID: workerID, JobID: job.JobID, Emiten: job.Announcement.Ticker}
				jobLogger.Info().Msg("successfully downloaded file")
				time.Sleep(time.Second * 7)
			}
		}
	}
}
