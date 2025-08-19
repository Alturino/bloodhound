package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	req "github.com/imroc/req/v3"

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
	logger := logging.Get()

	logger.Debug().Any("cookies", r.http.Cookies)

	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"
	pageStr := strconv.Itoa(page)
	pageSizeStr := strconv.Itoa(pageSize)
	result, err := r.http.R().
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
		return response.IdxResponse{}, err
	}

	if result.IsErrorState() {
		err = fmt.Errorf(
			"request failed with statusCode:%d error: %s",
			result.StatusCode,
			result.String(),
		)
		return response.IdxResponse{}, err
	}

	var res response.IdxResponse
	if err = result.UnmarshalJson(&res); err != nil {
		return response.IdxResponse{}, fmt.Errorf("failed UnmarshalJson with error: %w", err)
	}

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

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := path.Join(homeDir, "Downloads", "bloodhound", emiten)
	if err = os.MkdirAll(dir, os.FileMode(0o755)); err != nil {
		return err
	}

	filePath := filepath.Join(dir, filename)

	err = r.http.NewParallelDownload(attachment.FullSavePath).
		SetOutputFile(filePath).
		SetFileMode(os.FileMode(0o755)).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("repository failed to download file with error: %w", err)
	}

	log.Println("repository successfully downloaded file")

	return nil
}
