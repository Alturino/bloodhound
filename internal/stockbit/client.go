package stockbit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/models"
)

// Client handles API requests to Stockbit
type Client struct {
	httpClient *req.Client
	config     *config.StockbitConfig
	logger     *slog.Logger
	tracer     trace.Tracer
}

// NewClient creates a new Stockbit HTTP client
func NewClient(
	config *config.StockbitConfig,
	token string,
	logger *slog.Logger,
	tracer trace.Tracer,
) *Client {
	client := req.C().
		EnableAutoDecompress().
		ImpersonateChrome().
		SetTimeout(30*time.Second).
		SetCommonRetryCount(3).
		SetCommonRetryBackoffInterval(1*time.Second, 5*time.Second).
		SetCommonHeaders(map[string]string{
			"accept":             "application/json",
			"accept-language":    "en,en-US;q=0.9,id;q=0.8",
			"authorization":      "Bearer " + token,
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
		}).
		DisableAutoReadResponse()

	return &Client{
		httpClient: client,
		config:     config,
		logger:     logger,
		tracer:     tracer,
	}
}

// FetchMarketDetector fetches broker flow and summary from Stockbit API
func (c Client) FetchMarketDetector(
	ctx context.Context,
	symbol string,
	dateFrom string,
	dateTo string,
) (models.StockbitMarketDetectorResponse, error) {
	ctx, span := c.tracer.Start(ctx, "StockbitClient.FetchMarketDetector")
	defer span.End()

	c.logger.DebugContext(ctx, "fetching stockbit market detector",
		slog.String("symbol", symbol),
		slog.String("from", dateFrom),
		slog.String("to", dateTo),
	)

	// API Endpoint: https://exodus.stockbit.com/marketdetectors/{symbol}
	url := fmt.Sprintf("%s/marketdetectors/%s", c.config.BaseURL, symbol)

	resp, err := c.httpClient.R().
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
		err = fmt.Errorf("failed to fetch stockbit market detector: %w", err)
		return models.StockbitMarketDetectorResponse{}, err
	}

	if resp.StatusCode == 401 {
		c.logger.ErrorContext(ctx, "stockbit token unauthorized or expired")
		return models.StockbitMarketDetectorResponse{}, fmt.Errorf("stockbit unauthorized")
	}

	if !resp.IsSuccessState() {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return models.StockbitMarketDetectorResponse{}, err
	}

	var rawResp models.StockbitMarketDetectorResponse
	if err := json.Unmarshal(resp.Bytes(), &rawResp); err != nil {
		err = fmt.Errorf("failed to parse response: %w", err)
		return models.StockbitMarketDetectorResponse{}, err
	}

	c.logger.InfoContext(ctx, "fetched stockbit market detector",
		slog.String("symbol", symbol),
		slog.String("message", rawResp.Message),
	)

	return rawResp, nil
}
