package idx

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
)

type mockClient struct {
	logger *slog.Logger
	tracer trace.Tracer
}

func NewMockClient(logger *slog.Logger, tracer trace.Tracer) Client {
	return &mockClient{
		logger: logger,
		tracer: tracer,
	}
}

func (m mockClient) FetchAnnouncements(ctx context.Context, indexFrom int) (models.AnnouncementResponse, error) {
	ctx, span := m.tracer.Start(ctx, "MockClient.FetchAnnouncements")
	defer span.End()

	m.logger.DebugContext(ctx, "using mock data", slog.Int("index_from", indexFrom))

	now := time.Now()
	announcementDate := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	return models.AnnouncementResponse{
		ResultCount: 1,
		SearchParams: models.SearchParams{
			DateFrom:  now.AddDate(0, 0, -30).Format("2006-01-02"),
			DateTo:    now.Format("2006-01-02"),
			IndexFrom: indexFrom,
			PageSize:  10,
		},
		Replies: []models.Reply{
			{
				Announcement: models.Announcement{
					ID2:               "MOCK-001",
					ID:                1,
					AnnouncementTitle: "Mock Announcement for Development",
					AnnouncementDate:  announcementDate,
					StockCode:         "MOCK",
					CreatedDate:       now,
				},
			},
		},
	}, nil
}

func NewMockClientFromConfig(
	config *config.IDX,
	logger *slog.Logger,
	tracer trace.Tracer,
) Client {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "idx.MockClient"))
	}
	return NewMockClient(logger, tracer)
}
