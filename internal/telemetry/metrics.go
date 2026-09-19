package telemetry

import (
	"go.opentelemetry.io/otel/metric"
)

type MetricsProvider struct {
	AnnouncementsFetched           metric.Int64Counter
	AnnouncementsSaved             metric.Int64Counter
	AnnouncementProcessingDuration metric.Float64Histogram
	PagesFetched                   metric.Int64Counter
	PageFetchDuration              metric.Float64Histogram
	AttachmentDownload             metric.Int64Counter
	AttachmentDownloadDuration     metric.Float64Histogram
	AttachmentDownloadSize         metric.Int64Histogram
	AttachmentPolledCount          metric.Int64Gauge
}

func NewMetrics(meter metric.Meter) (*MetricsProvider, error) {
	idxAnnouncementsFetched, err := meter.Int64Counter(
		"bloodhound.idx.announcements.fetched",
		metric.WithDescription("Total announcements fetched from IDX API"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	idxAnnouncementsSaved, err := meter.Int64Counter(
		"bloodhound.idx.announcements.saved",
		metric.WithDescription("Announcements saved to DB"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	idxProcessingDuration, err := meter.Float64Histogram(
		"bloodhound.idx.processing.duration",
		metric.WithDescription("Full IDX processing cycle duration"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	idxPagesFetched, err := meter.Int64Counter(
		"bloodhound.idx.pages.fetched",
		metric.WithDescription("Pages fetched from IDX API"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	idxPageFetchDuration, err := meter.Float64Histogram(
		"bloodhound.idx.page.fetch.duration",
		metric.WithDescription("Single page fetch duration"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	attDownloadTotal, err := meter.Int64Counter(
		"bloodhound.attachment.download.total",
		metric.WithDescription("Total attachment downloads, status attribute"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	attDownloadDuration, err := meter.Float64Histogram(
		"bloodhound.attachment.download.duration",
		metric.WithDescription("Attachment download duration"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	attDownloadSize, err := meter.Int64Histogram(
		"bloodhound.attachment.download.size",
		metric.WithDescription("Attachment download size"),
		metric.WithUnit("bytes"),
	)
	if err != nil {
		return nil, err
	}

	attPolledCount, err := meter.Int64Gauge(
		"bloodhound.attachment.polled.count",
		metric.WithDescription("Unprocessed attachments found per poll"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	return &MetricsProvider{
		AnnouncementsFetched:           idxAnnouncementsFetched,
		AnnouncementsSaved:             idxAnnouncementsSaved,
		AnnouncementProcessingDuration: idxProcessingDuration,
		PagesFetched:                   idxPagesFetched,
		PageFetchDuration:              idxPageFetchDuration,
		AttachmentDownload:             attDownloadTotal,
		AttachmentDownloadDuration:     attDownloadDuration,
		AttachmentDownloadSize:         attDownloadSize,
		AttachmentPolledCount:          attPolledCount,
	}, nil
}
