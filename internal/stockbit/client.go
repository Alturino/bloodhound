package stockbit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type Client interface {
	FetchMarketDetector(
		ctx context.Context,
		symbol, dateFrom, dateTo string,
	) (MarketDetectorResponse, error)
	FetchBrokerActivity(
		ctx context.Context,
		symbol, dateFrom, dateTo string,
	) (BrokerActivityResponse, error)
}

// Client handles API requests to Stockbit
type client struct {
	httpclient *req.Client
	config     *config.Stockbit
	logger     *slog.Logger
	tracer     trace.Tracer
}

// NewClient creates a new Stockbit HTTP client
func NewClient(
	httpclient *req.Client,
	config *config.Stockbit,
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
		"accept":             "application/json",
		"accept-language":    "en,en-US;q=0.9,id;q=0.8",
		"authorization":      "Bearer " + config.Token,
		"dnt":                "1",
		"origin":             "https://stockbit.com",
		"priority":           "u=1, i",
		"referer":            "https://stockbit.com/",
		"sec-ch-ua":          `"Not:A-Brand";v="99", "Microsoft Edge";v="145", "Chromium";v="145"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Linux"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-site",
	}).SetBaseURL(config.BaseURL)

	return &client{
		httpclient: httpclient,
		config:     config,
		logger:     logger,
		tracer:     tracer,
	}
}

// FetchMarketDetector fetches broker flow and summary from Stockbit API
func (c *client) FetchMarketDetector(
	ctx context.Context,
	symbol, dateFrom, dateTo string,
) (MarketDetectorResponse, error) {
	ctx, span := c.tracer.Start(
		ctx,
		"stockbit.Client.FetchMarketDetector",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	logger := c.logger.With(
		slog.String("tag", "stockbit.Client.FetchMarketDetector"),
		slog.String("symbol", symbol),
		slog.String("from", dateFrom),
		slog.String("to", dateTo),
	)

	logger.DebugContext(ctx, "fetching stockbit market detector")
	span.AddEvent("fetching stockbit market detector")
	// API Endpoint: https://exodus.stockbit.com/marketdetectors/{symbol}
	var rawResp MarketDetectorResponse
	resp, err := c.httpclient.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"from":             dateFrom,
			"to":               dateTo,
			"transaction_type": "TRANSACTION_TYPE_GROSS",
			"market_board":     "MARKET_BOARD_REGULER",
			"investor_type":    "INVESTOR_TYPE_ALL",
			"limit":            "25",
		}).
		SetSuccessResult(&rawResp).
		Get("/marketdetectors/" + symbol)
	if err != nil {
		err = fmt.Errorf("fetching stockbit market detector: %v", err)
		return MarketDetectorResponse{}, err
	}
	if resp.IsErrorState() {
		err := fmt.Errorf("fetching stockbit market detector: status_code=%d", resp.StatusCode)
		return MarketDetectorResponse{}, err
	}
	span.AddEvent("fetched stockbit market detector")
	logger.DebugContext(ctx, "fetched stockbit market detector")

	return rawResp, nil
}

func (c *client) FetchBrokerActivity(
	ctx context.Context,
	symbol string,
	dateFrom string,
	dateTo string,
) (BrokerActivityResponse, error) {
	panic("not implemented") // TODO: Implement
}
