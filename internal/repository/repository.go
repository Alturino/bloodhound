package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	req "github.com/imroc/req/v3"

	"github.com/Alturino/bloodhound/internal/response"
)

type Repository struct {
	http *req.Client
}

func NewRepository(http *req.Client) Repository {
	return Repository{http: http}
}

func (r Repository) Get(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) (response.Response, error) {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"
	pageStr := strconv.Itoa(page)
	pageSizeStr := strconv.Itoa(pageSize)
	result, err := buildRequest(r.http.R()).
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
		return response.Response{}, fmt.Errorf("repository failed to get data with error: %w", err)
	}

	if result.IsErrorState() {
		err = fmt.Errorf(
			"request failed with statusCode:%d error: %s",
			result.StatusCode,
			result.String(),
		)
		return response.Response{}, err
	}

	var res response.Response
	if err = result.UnmarshalJson(&res); err != nil {
		return response.Response{}, fmt.Errorf("failed UnmarshalJson with error: %w", err)
	}

	return res, nil
}

func (r Repository) GetFile(
	ctx context.Context,
	attachment response.Attachment,
) (*req.Response, error) {
	exp := regexp.MustCompile(`[A-Z]{4}`)
	emiten := exp.FindString(attachment.OriginalFilename)
	filename := strings.TrimSpace(attachment.OriginalFilename)
	filename = strings.ReplaceAll(filename, " ", "_")
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ToLower(filename)
	fp := filepath.Join(emiten, filename)

	res, err := buildRequest(r.http.R()).
		// EnableDump().
		// EnableDumpTo(os.Stdout).
		SetOutputFile(fp).
		SetContext(ctx).
		Get(attachment.FullSavePath)
	if err != nil {
		return nil, fmt.Errorf("repository failed to get data with error: %w", err)
	}

	if res.IsErrorState() {
		return nil, fmt.Errorf(
			"request failed with statusCode: %d error: %s",
			res.StatusCode,
			res.String(),
		)
	}

	return res, nil
}

func buildRequest(request *req.Request) *req.Request {
	return request.SetHeader("Accept-Encoding", "gzip").
		SetHeader("Connection", "keep-alive").
		SetHeader("Host", "idx.co.id").
		SetHeader("Referer", "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/").
		SetHeader("Sec-Fetch-Dest", "document").
		SetHeader("Sec-Fetch-Mode", "navigate").
		SetHeader("Sec-Fetch-Site", "cross-site").
		SetHeader("sec-ch-ua-platform", `"Android"`)
}
