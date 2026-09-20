package stockbit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type Client interface {
	FetchMarketDetector(
		ctx context.Context,
		symbol, dateFrom, dateTo string,
	) (models.StockbitMarketDetectorResponse, error)
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
		"user-agent":         "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36 Edg/145.0.0.0",
	}).SetBaseURL(config.BaseURL)

	return &client{
		httpclient: httpclient,
		config:     config,
		logger:     logger,
		tracer:     tracer,
	}
}

// FetchMarketDetector fetches broker flow and summary from Stockbit API
func (c client) FetchMarketDetector(
	ctx context.Context,
	symbol, dateFrom, dateTo string,
) (models.StockbitMarketDetectorResponse, error) {
	ctx, span := c.tracer.Start(ctx, "stockbit.Client.FetchMarketDetector")
	defer span.End()

	logger := c.logger.With(
		slog.String("tag", "stockbit.Client.FetchMarketDetector"),
		slog.String(constants.Symbol, symbol),
		slog.String(constants.DateFrom, dateFrom),
		slog.String(constants.DateTo, dateTo),
	)

	logger.DebugContext(ctx, "fetching stockbit market detector")
	span.AddEvent("fetching stockbit market detector")
	// API Endpoint: https://exodus.stockbit.com/marketdetectors/{symbol}
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
		Get("/marketdetectors/" + symbol)
	if err != nil {
		err = fmt.Errorf("fetching stockbit market detector: %w", err)
		telemetry.RecordError(span, err)
		return models.StockbitMarketDetectorResponse{}, err
	}
	if !resp.IsSuccessState() {
		err = fmt.Errorf("fetching stockbit market detector msg: status code: %d", resp.StatusCode)
		telemetry.RecordError(span, err)
		return models.StockbitMarketDetectorResponse{}, err
	}
	logger.DebugContext(ctx, "fetched stockbit market detector")
	span.AddEvent("fetched stockbit market detector")

	logger.DebugContext(ctx, "unmarshaling response")
	span.AddEvent("unmarshaling response")
	var rawResp models.StockbitMarketDetectorResponse
	if err := json.Unmarshal(resp.Bytes(), &rawResp); err != nil {
		err = fmt.Errorf("unmarshaling response: %w", err)
		telemetry.RecordError(span, err)
		return models.StockbitMarketDetectorResponse{}, err
	}
	logger.DebugContext(ctx, "unmarshaled response")
	span.AddEvent("unmarshaled response")

	return rawResp, nil
}
