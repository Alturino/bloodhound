package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	req "github.com/imroc/req/v3"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/response"
)

type HTTPRepository struct {
	http *req.Client
}

func NewHTTPRepository(http *req.Client) HTTPRepository {
	return HTTPRepository{http: http}
}

func (r HTTPRepository) Get(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) (response.IdxResponse, error) {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"

	logger := zerolog.Ctx(ctx).With().
		Str(logging.KEY_TAG, "HTTPRepository Get").
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
	logger = logger.With().Any("idx_response", res).Logger()
	logger.Info().Msg("repository successfully get data")

	return res, nil
}

func (r HTTPRepository) DownloadFile(
	ctx context.Context,
	emiten string,
	attachment response.Attachment,
) error {
	filename := strings.ReplaceAll(attachment.OriginalFilename, "/", " ")
	filename = strings.TrimSpace(filename)

	re := regexp.MustCompile(`\s+`)
	filename = re.ReplaceAllString(filename, " ")
	filename = strings.ReplaceAll(filename, " ", "_")

	emitenDir := filepath.Join(common.BloodhoundDir, emiten)
	downloadPath := filepath.Join(emitenDir, filename)
	logger := zerolog.Ctx(ctx).
		With().
		Str(logging.KEY_TAG, "HTTPRepository DownloadFile").
		Str("url", attachment.FullSavePath).
		Str("filename", filename).
		Str("bloodhound_dir", emitenDir).
		Str("downloaded_path", downloadPath).
		Logger()

	logger.Debug().Msg("creating directory")
	if err := os.MkdirAll(emitenDir, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("repository failed to create directory with error: %w", err)
		logger.Error().Err(err).Msg(err.Error())
		return err
	}
	logger.Debug().Msg("directory created")

	file, err := os.OpenFile(downloadPath, os.O_CREATE|os.O_WRONLY, os.FileMode(0o755))
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
