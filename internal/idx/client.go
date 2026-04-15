package idx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// IDXClient defines the subset of IDX client methods needed by the worker
type Client interface {
	FetchAnnouncements(ctx context.Context, indexFrom int) (models.AnnouncementResponse, error)
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
		logger = slog.Default().With(slog.String("tag", "stockbit.Client"))
	}
	if tracer == nil {
		tracer = telemetry.AppTelemetry.Tracer
	}
	httpclient = httpclient.Clone().SetCommonHeaders(map[string]string{
		"Connection":         "keep-alive",
		"Accept-Encoding":    "gzip",
		"Host":               "idx.co.id",
		"Referer":            "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/",
		"Sec-Fetch-Dest":     "document",
		"Sec-Ch-Ua":          `"Chromium";v="139", "Not;A=Brand";v="99"`,
		"Sec-Fetch-Mode":     "navigate",
		"Sec-Fetch-Site":     "none",
		"sec-ch-ua-platform": `"Linux"`,
	}).SetBaseURL(config.BaseURL)

	return &client{
		httpclient: httpclient,
		config:     config,
		logger:     logger,
		tracer:     tracer,
	}
}

// FetchAnnouncements fetches announcements from IDX API
func (c client) FetchAnnouncements(
	ctx context.Context,
	indexFrom int,
) (models.AnnouncementResponse, error) {
	ctx, span := c.tracer.Start(ctx, "HTTPClient.FetchAnnouncements")
	defer span.End()

	logger := c.logger.With(
		slog.Int("index_from", indexFrom),
		slog.Int("page_size", c.config.PageSize),
	)

	logger.DebugContext(ctx, "fetching announcements")
	span.AddEvent("fetching announcements")
	resp, err := c.httpclient.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"indexfrom": fmt.Sprintf("%d", indexFrom),
			"pagesize":  fmt.Sprintf("%d", c.config.PageSize),
		}).
		Get("/primary/ListedCompany/GetAnnouncement")
	if err != nil {
		err = fmt.Errorf("fetching announcements: %w", err)
		telemetry.RecordError(span, err)
		return models.AnnouncementResponse{}, err
	}
	if !resp.IsSuccessState() {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		telemetry.RecordError(span, err)
		return models.AnnouncementResponse{}, err
	}
	logger.InfoContext(ctx, "fetched announcements")
	span.AddEvent("fetched announcements")

	logger.DebugContext(ctx, "unmarshaling response")
	span.AddEvent("unmarshaling response")
	var rawResp rawAnnouncementResponse
	if err := json.Unmarshal(resp.Bytes(), &rawResp); err != nil {
		err = fmt.Errorf("unmarshaling response: %w", err)
		return models.AnnouncementResponse{}, err
	}
	result := c.convertToModel(rawResp)
	logger.DebugContext(ctx, "unmarshaled response")
	span.AddEvent("unmarshaled response")

	logger.InfoContext(ctx, "fetched announcements",
		slog.Int("result_count", result.ResultCount),
		slog.Int("replies_count", len(result.Replies)),
	)

	return result, nil
}

// DownloadFile downloads a file from given URL
func (c client) DownloadFile(ctx context.Context, url string) ([]byte, string, error) {
	ctx, span := c.tracer.Start(
		ctx,
		"HTTPClient.DownloadFile",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("url", url)),
	)
	defer span.End()

	logger := c.logger.With(slog.String("url", url), slog.String("tag", "idx.Client.DownloadFile"))

	logger.DebugContext(ctx, "downloading file")
	span.AddEvent("downloading file")
	resp, err := c.httpclient.R().
		SetContext(ctx).
		Get(url)
	if err != nil {
		err = fmt.Errorf("downloading file: %w", err)
		telemetry.RecordError(span, err)
		return nil, "", err
	}
	if !resp.IsSuccessState() {
		err := fmt.Errorf("unexpected status code: %d, url: %s", resp.StatusCode, url)
		return nil, "", err
	}
	logger.DebugContext(ctx, "downloaded file")
	span.AddEvent("downloaded file")

	contentType := resp.Header.Get("Content-Type")
	data := resp.Bytes()

	logger.InfoContext(ctx, "downloaded file",
		slog.String("url", url),
		slog.Int("size", len(data)),
		slog.String("content_type", contentType),
	)

	return data, contentType, nil
}

// convertToModel converts raw API response to our model
func (c client) convertToModel(raw rawAnnouncementResponse) models.AnnouncementResponse {
	result := models.AnnouncementResponse{
		ResultCount: raw.ResultCount,
		SearchParams: models.SearchParams{
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
		Replies: make([]models.Reply, 0, len(raw.Replies)),
	}

	for _, r := range raw.Replies {
		reply := models.Reply{
			Announcement: models.Announcement{
				ID2:                 r.Pengumuman.Id2,
				ID:                  r.Pengumuman.ID,
				FinalID:             r.Pengumuman.FinalId,
				OldFinalID:          r.Pengumuman.OldFinalId,
				AnnouncementNumber:  r.Pengumuman.NoPengumuman,
				AnnouncementDate:    r.Pengumuman.TglPengumuman.Time(),
				AnnouncementTitle:   r.Pengumuman.JudulPengumuman,
				AnnouncementType:    r.Pengumuman.JenisPengumuman,
				StockCode:           strings.TrimSpace(r.Pengumuman.Kode_Emiten),
				CreatedDate:         r.Pengumuman.CreatedDate.Time(),
				FormID:              r.Pengumuman.Form_Id,
				AnnouncementSubject: r.Pengumuman.PerihalPengumuman,
				JMSXGroupID:         r.Pengumuman.JMSXGroupID,
				Division:            r.Pengumuman.Divisi,
				DivisionCode:        r.Pengumuman.KodeDivisi,
				StockTypeDetail:     r.Pengumuman.JenisEmiten,
				IsStock:             r.Pengumuman.EfekEmiten_Saham,
				IsBond:              r.Pengumuman.EfekEmiten_Obligasi,
				IsEBA:               r.Pengumuman.EfekEmiten_EBA,
				IsETF:               r.Pengumuman.EfekEmiten_ETF,
				IsSPEI:              r.Pengumuman.EfekEmiten_SPEI,
				Attachments:         make([]models.Attachment, 0, len(r.Attachments)),
			},
		}

		for _, a := range r.Attachments {
			reply.Announcement.Attachments = append(
				reply.Announcement.Attachments,
				models.Attachment{
					ID:               a.ID,
					PDFFilename:      a.PDFFilename,
					FullSavePath:     a.FullSavePath,
					JMSXGroupID:      a.JMSXGroupID,
					CorrelationID:    a.CorrelationID,
					IsAttachment:     a.IsAttachment,
					OriginalFilename: a.OriginalFilename,
				},
			)
		}

		result.Replies = append(result.Replies, reply)
	}

	return result
}
