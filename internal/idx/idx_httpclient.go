package idx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/imroc/req/v3"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// IDXClient defines the subset of IDX client methods needed by the worker
type Client interface {
	FetchAnnouncements(
		ctx context.Context,
		page int,
		dateFrom time.Time,
	) (AnnouncementResponse, error)
	DownloadFile(ctx context.Context, url string) ([]byte, string, error)
}

// Client handles API requests to IDX
type client struct {
	httpclient *req.Client
	config     *config.IDX
	logger     *slog.Logger
	tracer     trace.Tracer
}

// NewClient creates a new IDX HTTP client with Chrome browser impersonation
func NewClient(
	httpclient *req.Client,
	config *config.IDX,
	logger *slog.Logger,
	tracer trace.Tracer,
) Client {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "idx.Client"))
	}
	if tracer == nil {
		tracer = telemetry.AppTelemetry.Tracer
	}
	idxhttpclient := httpclient.SetCommonHeaders(map[string]string{
		"Connection":         "keep-alive",
		"Accept-Encoding":    "gzip",
		"Host":               "idx.co.id",
		"Referer":            "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/",
		"Sec-Fetch-Dest":     "document",
		"Sec-Ch-Ua":          `"Chromium";v="139", "Not;A=Brand";v="99"`,
		"Sec-Fetch-Mode":     "navigate",
		"Sec-Fetch-Site":     "none",
		"sec-ch-ua-platform": `"Linux"`,
	}).
		SetBaseURL(config.BaseURL)

	return &client{
		httpclient: idxhttpclient,
		config:     config,
		logger:     logger,
		tracer:     tracer,
	}
}

// FetchAnnouncements fetches announcements from IDX API
func (c *client) FetchAnnouncements(
	ctx context.Context,
	page int,
	dateFrom time.Time,
) (AnnouncementResponse, error) {
	now := time.Now()
	ctx, span := c.tracer.Start(
		ctx,
		"idx.client.FetchAnnouncements",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	ctx = slogctx.Append(
		ctx,
		slog.Time(constants.DateFrom, dateFrom),
		slog.Time(constants.DateTo, now),
	)
	logger := c.logger.With(slog.String("tag", "idx.client.FetchAnnouncements"))

	if dateFrom.IsZero() {
		dt, err := time.Parse("20060102", "19010101")
		if err != nil {
			err = fmt.Errorf("parsing default date: %w", err)
			telemetry.RecordError(span, err)
			return AnnouncementResponse{}, err
		}
		dateFrom = dt
	}

	logger.DebugContext(ctx, "fetching announcements")
	span.AddEvent("fetching announcements")
	var rawResp rawAnnouncementResponse
	req := c.httpclient.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"indexFrom": fmt.Sprintf("%d", page),
			"pageSize":  fmt.Sprintf("%d", c.config.PageSize),
			"dateFrom":  dateFrom.Format("20060102"),
			"dateTo":    now.Format("20060102"),
		}).
		SetSuccessResult(&rawResp).
		EnableDump()
	logger = logger.With(slog.String(constants.URL, req.RawURL))
	resp, err := req.Get("/primary/ListedCompany/GetAnnouncement")
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.String(constants.HTTPDump, resp.Dump()))
	}
	logger = logger.With(slog.String("request_url", resp.Request.URL.String()))
	if err != nil {
		err = fmt.Errorf("fetching announcements: %w", err)
		telemetry.RecordError(span, err)
		return AnnouncementResponse{}, err
	}
	if resp.IsErrorState() {
		err := fmt.Errorf("fetching announcements: unexpected status_code=%d", resp.StatusCode)
		telemetry.RecordError(span, err)
		return AnnouncementResponse{}, err
	}
	if len(rawResp.Replies) == 0 {
		err := errors.New("announcements empty")
		telemetry.RecordError(span, err)
		return AnnouncementResponse{}, err
	}
	result := convertToModel(rawResp)
	if len(result.Announcements) == 0 {
		telemetry.RecordError(span, errors.New("announcements empty"))
		return AnnouncementResponse{}, errors.New("announcements empty")
	}
	logger = logger.With(slog.Int(constants.AnnouncementsCount, len(result.Announcements)))
	logger.DebugContext(ctx, "fetched announcements")
	span.AddEvent("fetched announcements")

	return result, nil
}

// DownloadFile downloads a file from given URL
func (c *client) DownloadFile(ctx context.Context, url string) ([]byte, string, error) {
	ctx, span := c.tracer.Start(
		ctx,
		"idx.client.DownloadFile",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	logger := c.logger.With(slog.String("tag", "idx.client.DownloadFile"))

	logger.DebugContext(ctx, "downloading file")
	span.AddEvent("downloading file")
	resp, err := c.httpclient.R().
		EnableDumpWithoutResponseBody().
		SetContext(ctx).
		Get(url)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.String(constants.HTTPDump, resp.Dump()))
	}
	if err != nil {
		err = fmt.Errorf("downloading file: %w", err)
		telemetry.RecordError(span, err)
		return nil, "", err
	}
	if resp.IsErrorState() {
		err := fmt.Errorf(
			"downloading file: unexpected status_code=%d, url=%s",
			resp.StatusCode,
			url,
		)
		telemetry.RecordError(span, err)
		return nil, "", err
	}
	logger.DebugContext(ctx, "downloaded file")
	span.AddEvent("downloaded file")

	contentType := resp.Header.Get("Content-Type")
	data := resp.Bytes()

	return data, contentType, nil
}

func convertToModel(raw rawAnnouncementResponse) AnnouncementResponse {
	result := AnnouncementResponse{
		ResultCount: raw.ResultCount,
		SearchParams: SearchParams{
			DateFrom:   raw.SearchParams.DateFrom,
			DateTo:     raw.SearchParams.DateTo,
			Query:      raw.SearchParams.Query,
			Language:   raw.SearchParams.Language,
			StockCode:  raw.SearchParams.KodeEmiten,
			StockType:  raw.SearchParams.EmitenType,
			SortOrder:  raw.SearchParams.SortOrder,
			SortColumn: raw.SearchParams.SortColumn,
			IndexFrom:  raw.SearchParams.IndexFrom,
			PageSize:   raw.SearchParams.PageSize,
		},
		Announcements: make([]Announcement, len(raw.Replies)),
	}

	for i, r := range raw.Replies {
		stockcode := strings.ReplaceAll(r.Pengumuman.KodeEmiten, " ", "")
		stockcode = strings.ReplaceAll(stockcode, "//", "")
		title := strings.ReplaceAll(r.Pengumuman.JudulPengumuman, "//", "")
		announcement := Announcement{
			ID:                r.Pengumuman.Id2,
			Date:              r.Pengumuman.TglPengumuman.Time(),
			AnnouncementTitle: title,
			AnnouncementType:  r.Pengumuman.JenisPengumuman,
			StockCode:         stockcode,
			CreatedDate:       r.Pengumuman.CreatedDate.Time(),
			IsStock:           r.Pengumuman.EfekEmiten_Saham,
			Attachments:       make([]Attachment, 0, len(r.Attachments)),
		}
		for _, attachment := range r.Attachments {
			candidate := Attachment{
				PDFFilename:      attachment.PDFFilename,
				FullSavePath:     attachment.FullSavePath,
				OriginalFilename: attachment.OriginalFilename,
			}
			if !candidate.isPDF() {
				continue
			}
			announcement.Attachments = append(announcement.Attachments, candidate)
		}
		announcement.Attachments = slices.Clip(announcement.Attachments)
		result.Announcements[i] = announcement
	}

	return result
}
