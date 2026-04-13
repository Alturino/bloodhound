package stockbit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
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
	if tracer == nil {
		tracer = telemetry.AppTelemetry.Tracer
	}
	httpclient = httpclient.
		SetCommonHeaders(map[string]string{
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
	ctx, span := c.tracer.Start(ctx, "StockbitClient.FetchMarketDetector")
	defer span.End()

	c.logger.DebugContext(ctx, "fetching stockbit market detector",
		slog.String("symbol", symbol),
		slog.String("from", dateFrom),
		slog.String("to", dateTo),
	)

	// API Endpoint: https://exodus.stockbit.com/marketdetectors/{symbol}
	url := "/marketdetectors/" + symbol
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
		Get(url)
	if err != nil {
		err = fmt.Errorf("fetch stockbit market detector: %w", err)
		return models.StockbitMarketDetectorResponse{}, err
	}

	if resp.StatusCode == 401 {
		err = fmt.Errorf("stockbit token unauthorized or expired: %w", err)
		return models.StockbitMarketDetectorResponse{}, err
	}

	if !resp.IsSuccessState() {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return models.StockbitMarketDetectorResponse{}, err
	}

	var rawResp models.StockbitMarketDetectorResponse
	if err := json.Unmarshal(resp.Bytes(), &rawResp); err != nil {
		err = fmt.Errorf("unmarshal response: %w", err)
		return models.StockbitMarketDetectorResponse{}, err
	}

	c.logger.InfoContext(ctx, "fetched stockbit market detector",
		slog.String("symbol", symbol),
		slog.String("message", rawResp.Message),
	)

	return rawResp, nil
}
