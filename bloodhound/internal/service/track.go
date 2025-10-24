package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/response"
	"github.com/Alturino/bloodhound/internal/worker"
)

type Track struct {
	repo             *repository.HTTPRepository
	pool             int
	downloadJobCh    chan jobs.DownloadAttachmentArgs
	resDownloadJobCh chan jobs.DownloadRes
	stopDownloadCh   chan struct{}
}

var (
	once  sync.Once
	track Track
)

func NewTrack(
	ctx context.Context,
	repository *repository.HTTPRepository,
	pool int,
	downloadJobCh chan jobs.DownloadAttachmentArgs,
	resDownloadJobCh chan jobs.DownloadRes,
	stopDownloadCh chan struct{},
) *Track {
	once.Do(func() {
		track = Track{
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
			downloadWorkerFunc(),
		)
	})
	return &track
}

func (t Track) TrackTillEmpty(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dir := path.Join(common.BloodhoundDir, emiten, "responses")

	logger := zerolog.Ctx(ctx).
		With().
		Str(constants.KEY_TAG, "Track TrackTillEmpty").
		Str("search_emiten", emiten).
		Str("keyword", keyword).
		Str("dir", dir).
		Int("starting_page", page).
		Int("pageSize", pageSize).
		Logger()

	logger.Debug().Msg("creating directory")
	if err := os.MkdirAll(dir, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("failed to create directory with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("directory created")

	currentPage := page
	responses := make([]response.IdxResponse, 0, 100)
	for {
		pageLogger := logger.With().Int("page", currentPage).Logger()
		res, err := t.repo.Get(ctx, emiten, keyword, currentPage, pageSize)
		currentPage++
		if err != nil {
			err = fmt.Errorf("failed to get announcement with error: %w", err)
			pageLogger.Error().Err(err).Msg(err.Error())
			continue
		}
		if len(res.Replies) == 0 {
			err = errors.New("replies is empty, stopping")
			pageLogger.Error().Err(err).Msg(err.Error())
			break
		}
		go func() {
			for downloadRes := range t.resDownloadJobCh {
				resLogger := logger.With().
					Str("job_id", downloadRes.JobID).
					Str("emiten", downloadRes.Emiten).
					Int("attachment_id", downloadRes.AttachmentID).
					Int("worker_id", downloadRes.WorkerID).
					Logger()
				resLogger.Debug().Msg("received download result")
				if downloadRes.Err != nil {
					resLogger.Error().Err(downloadRes.Err).Msg(downloadRes.Err.Error())
					continue
				}
				resLogger.Info().Msg("successfully downloaded file")
			}
		}()
		for _, reply := range res.Replies {
			if len(emiten) == 0 {
				re := regexp.MustCompile(`\s+`)
				emiten = re.ReplaceAllString(reply.Announcement.Ticker, "")
				emiten = strings.TrimSpace(emiten)
				pageLogger.Debug().
					Str("emiten", emiten).
					Msg("emiten is empty taking from reply then extract it ")
			}
			for _, attachment := range reply.Attachments {
				jobID := uuid.NewString()
				pageLogger = pageLogger.With().
					Str("job_id", jobID).
					Str("job_emiten", emiten).
					Logger()
				pageLogger.Debug().Msg("sending job")
				t.downloadJobCh <- jobs.DownloadAttachmentArgs{JobID: jobID, Emiten: emiten, Attachment: attachment, AnnouncementDate: reply.Announcement.Date}
				pageLogger.Info().Msg("job sent")
			}
		}
		responses = append(responses, res)
		log.Println("successfully appending to responses")
	}

	timestamp := time.Now().Format("2006-01-02_15:04:05")
	filename := fmt.Sprintf("%s_responses.json", timestamp)
	filePath := filepath.Join(dir, filename)
	logger.Debug().Str("filepath", filePath).Msg("creating file")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, os.FileMode(0o755))
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
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, os.FileMode(0o755))
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
