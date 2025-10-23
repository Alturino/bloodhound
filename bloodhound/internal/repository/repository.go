package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/client"
	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/otel/otelutil"
	"github.com/Alturino/bloodhound/internal/response"
)

type HTTPRepository struct {
	http client.HTTPClient
}

func NewHTTPRepository(http client.HTTPClient) *HTTPRepository {
	return &HTTPRepository{http: http}
}

func (r HTTPRepository) Get(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) (response.IdxResponse, error) {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"

	logger := zerolog.Ctx(ctx).With().
		Str(constants.KEY_TAG, "HTTPRepository Get").
		Str("emiten", emiten).
		Str("keyword", keyword).
		Str("url", url).
		Int("page", page).
		Int("pageSize", pageSize).
		Logger()

	pageStr := strconv.Itoa(page)
	pageSizeStr := strconv.Itoa(pageSize)
	logger.Debug().Msg("repository getting data")
	resp, err := r.http.R().
		// EnableDump().
		// EnableDumpTo(os.Stdout).
		SetContext(ctx).
		AddQueryParam("kodeEmiten", emiten).
		AddQueryParam("indexFrom", pageStr).
		AddQueryParam("pageSize", pageSizeStr).
		AddQueryParam("lang", "id").
		AddQueryParam("emitenType", "*").
		AddQueryParam("keyword", keyword).
		Get(url)
	if err != nil {
		err = fmt.Errorf("repository failed to get data with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return response.IdxResponse{}, err
	}

	if resp.IsErrorState() {
		err = fmt.Errorf("request failed with statusCode: %d error: %w", resp.StatusCode, resp.Err)
		logger.Error().Err(err).Msg(err.Error())
		return response.IdxResponse{}, err
	}
	logger.Debug().Msg("repository successfully get data")

	var res response.IdxResponse
	if err = resp.UnmarshalJson(&res); err != nil {
		return response.IdxResponse{}, fmt.Errorf("failed UnmarshalJson with error: %w", err)
	}
	logger = logger.With().Any("idx_response", res).Any("cookies", resp.Cookies()).Logger()
	logger.Info().Msg("repository successfully get data")

	r.http.SetCommonCookies(resp.Request.Cookies...)

	return res, nil
}

func (r HTTPRepository) DownloadFile(
	ctx context.Context,
	emiten string,
	attachment response.Attachment,
	announcementDate time.Time,
) error {
	ctx, span := otelutil.Tracer.Start(ctx, "DownloadFile")
	defer span.End()

	filename := strings.ReplaceAll(attachment.OriginalFilename, "/", " ")
	filename = strings.TrimSpace(filename)

	excessSpaceRe := regexp.MustCompile(`\s+`)
	filename = excessSpaceRe.ReplaceAllString(filename, " ")
	filename = strings.ReplaceAll(filename, " ", "_")

	removeDateRe := regexp.MustCompile(`\d{8}`)
	filename = removeDateRe.ReplaceAllString(filename, "")
	filename = strings.ReplaceAll(filename, " ", "_")

	emitenDir := filepath.Join(common.BloodhoundDir, emiten)
	strDate := announcementDate.Format("2006_01_02")
	filename = strings.Join([]string{strDate, filename}, "_")
	downloadedFilepath := filepath.Join(emitenDir, filename)

	logger := zerolog.Ctx(ctx).
		With().
		Str(constants.KEY_TAG, "HTTPRepository DownloadFile").
		Str("url", attachment.FullSavePath).
		Str("filename", filename).
		Str("downloaded_path", downloadedFilepath).
		Logger()

	err := os.MkdirAll(emitenDir, os.FileMode(0o755))
	if err != nil {
		logger.Error().Err(err).Msg(err.Error())
		return err
	}

	logger.Debug().Msg("creating directory")
	if err := os.MkdirAll(emitenDir, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("repository failed to create directory with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("directory created")

	file, err := os.OpenFile(downloadedFilepath, os.O_CREATE|os.O_WRONLY, os.FileMode(0o755))
	if err != nil {
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	defer file.Close()
	logger.Debug().Msg("file created")

	logger.Debug().Msg("downloading file")
	err = r.http.NewParallelDownload(attachment.FullSavePath).
		SetOutput(file).
		Do(ctx)
	if err != nil {
		err = fmt.Errorf("repository failed to download file with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}

	logger.Info().Msg("repository successfully download file")

	return nil
}

func (r HTTPRepository) DownloadAnnouncement(
	ctx context.Context,
	announcement response.Reply,
) error {
	ctx, span := otelutil.Tracer.Start(ctx, "DownloadAnnouncement")
	defer span.End()

	logger := zerolog.Ctx(ctx).
		With().
		Ctx(ctx).
		Str(constants.KEY_TAG, "HTTPRepository DownloadAnnouncement").
		Str("announcement_title", announcement.Pengumuman.JudulPengumuman).
		Str("ticker", announcement.Pengumuman.KodeEmiten).
		Time("announcement_date", announcement.Pengumuman.TglPengumuman).
		Logger()

	var err error
	var wg sync.WaitGroup
	var mutex sync.Mutex
	for i, attachment := range announcement.Attachments {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer mutex.Unlock()
			lg := logger.With().Int("attachment_i", i+1).Logger()
			nCtx := lg.WithContext(ctx)
			downloadErr := r.DownloadFile(
				nCtx,
				announcement.Pengumuman.KodeEmiten,
				attachment,
				announcement.Pengumuman.TglPengumuman,
			)
			if downloadErr != nil {
				lg.Error().Err(downloadErr).Msg(downloadErr.Error())
			}
			mutex.Lock()
			err = errors.Join(downloadErr)
		}()
	}
	wg.Wait()

	if err != nil {
		err = fmt.Errorf("repository failed to download attachment with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	return nil
}
