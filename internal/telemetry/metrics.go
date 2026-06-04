package telemetry

import (
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	IdxAnnouncementsFetched      metric.Int64Counter
	IdxAnnouncementsSaved        metric.Int64Counter
	IdxAnnouncementsDuplicate    metric.Int64Counter
	IdxAnnouncementsFetchedTotal metric.Int64Gauge
	IdxProcessingDuration        metric.Float64Histogram
	IdxPagesFetched              metric.Int64Counter
	IdxPageFetchDuration         metric.Float64Histogram
	AttDownloaded                metric.Int64Counter
	AttFailedDownload            metric.Int64Counter
	AttDownloadDuration          metric.Float64Histogram
	AttDownloadSize              metric.Int64Histogram
	AttPolledCount               metric.Int64Gauge
	SbSymbolsSynced              metric.Int64Counter
	SbSyncDuration               metric.Float64Histogram
	SbStockCodesTotal            metric.Int64Gauge
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
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

	idxAnnouncementsDuplicate, err := meter.Int64Counter(
		"bloodhound.idx.announcements.duplicate",
		metric.WithDescription("Duplicate announcements skipped"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	idxAnnouncementsFetchedTotal, err := meter.Int64Gauge(
		"bloodhound.idx.announcements.total",
		metric.WithDescription("Total announcement count in API response gauge"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	idxProcessingDuration, err := meter.Float64Histogram(
		"bloodhound.idx.processing.duration",
		metric.WithDescription("Full IDX processing cycle duration (ms)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	idxPagesFetched, err := meter.Int64Counter(
		"bloodhound.idx.pages.fetched",
		metric.WithDescription("Pages fetched from IDX API, status attribute"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	idxPageFetchDuration, err := meter.Float64Histogram(
		"bloodhound.idx.page.fetch.duration",
		metric.WithDescription("Single page fetch duration (ms)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	attDownloaded, err := meter.Int64Counter(
		"bloodhound.attachment.downloaded",
		metric.WithDescription("Attachments downloaded, status attribute"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	attFailedDownload, err := meter.Int64Counter(
		"bloodhound.attachment.failed.download",
		metric.WithDescription("Attachments downloaded, status attribute"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	attDownloadDuration, err := meter.Float64Histogram(
		"bloodhound.attachment.download.duration",
		metric.WithDescription("Attachment download duration (ms)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	attDownloadSize, err := meter.Int64Histogram(
		"bloodhound.attachment.download.size",
		metric.WithDescription("Attachment download size (bytes)"),
		metric.WithUnit("bytes"),
	)
	if err != nil {
		return nil, err
	}

	attPolledCount, err := meter.Int64Gauge(
		"bloodhound.attachment.polled.count",
		metric.WithDescription("Unprocessed attachments found per poll gauge"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	sbSymbolsSynced, err := meter.Int64Counter(
		"bloodhound.stockbit.symbols.synced",
		metric.WithDescription("Stockbit symbols synced, status attribute"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	sbSyncDuration, err := meter.Float64Histogram(
		"bloodhound.stockbit.sync.duration",
		metric.WithDescription("Stockbit sync duration per symbol (ms)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	sbStockCodesTotal, err := meter.Int64Gauge(
		"bloodhound.stockbit.symbols.total",
		metric.WithDescription("Total stock codes to sync gauge"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		IdxAnnouncementsFetched:      idxAnnouncementsFetched,
		IdxAnnouncementsSaved:        idxAnnouncementsSaved,
		IdxAnnouncementsDuplicate:    idxAnnouncementsDuplicate,
		IdxAnnouncementsFetchedTotal: idxAnnouncementsFetchedTotal,
		IdxProcessingDuration:        idxProcessingDuration,
		IdxPagesFetched:              idxPagesFetched,
		IdxPageFetchDuration:         idxPageFetchDuration,
		AttDownloaded:                attDownloaded,
		AttFailedDownload:            attFailedDownload,
		AttDownloadDuration:          attDownloadDuration,
		AttDownloadSize:              attDownloadSize,
		AttPolledCount:               attPolledCount,
		SbSymbolsSynced:              sbSymbolsSynced,
		SbSyncDuration:               sbSyncDuration,
		SbStockCodesTotal:            sbStockCodesTotal,
	}, nil
}
