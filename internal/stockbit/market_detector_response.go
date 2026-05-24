package stockbit

import (
	"github.com/shopspring/decimal"
)

// MarketDetectorResponse represents the JSON response from Stockbit
type MarketDetectorResponse struct {
	Message string     `json:"message"`
	Data    MarketData `json:"data"`
}

type MarketData struct {
	From           string         `json:"from"`
	To             string         `json:"to"`
	BandarDetector BandarDetector `json:"bandar_detector"`
	BrokerSummary  BrokerSummary  `json:"broker_summary"`
}

type BandarDetector struct {
	NumberBrokerBuySell int             `json:"number_broker_buysell"`
	TotalBuyer          int             `json:"total_buyer"`
	TotalSeller         int             `json:"total_seller"`
	Value               decimal.Decimal `json:"value"`
	Volume              decimal.Decimal `json:"volume"`
	Average             decimal.Decimal `json:"average"`
	BrokerAccDist       string          `json:"broker_accdist"`
	Top1                AccDistLevel    `json:"top1"`
	Top3                AccDistLevel    `json:"top3"`
	Top5                AccDistLevel    `json:"top5"`
	Top10               AccDistLevel    `json:"top10"`
	Avg                 AccDistLevel    `json:"avg"`
	Avg5                AccDistLevel    `json:"avg5"`
}

type AccDistLevel struct {
	AccDist string          `json:"accdist"`
	Amount  decimal.Decimal `json:"amount"`
	Percent decimal.Decimal `json:"percent"`
	Vol     decimal.Decimal `json:"vol"`
}

type BrokerSummary struct {
	Symbol      string              `json:"symbol"`
	BrokersBuy  []BrokerTransaction `json:"brokers_buy"`
	BrokersSell []BrokerTransaction `json:"brokers_sell"`
}

type BrokerTransaction struct {
	Freq         int64           `json:"freq,string"`
	BrokerCode   string          `json:"net_broker_code"`
	StockCode    string          `json:"netbs_stock_code"`
	Date         string          `json:"netbs_date"`
	InvestorType string          `json:"type"`
	BuyAvgPrice  decimal.Decimal `json:"netbs_buy_avg_price"`
	SellAvgPrice decimal.Decimal `json:"netbs_sell_avg_price"`
	BLot         decimal.Decimal `json:"blot"`
	SLot         decimal.Decimal `json:"slot"`
}

// Internal Domain wrappers
type MarketDetectorSummary struct {
	Symbol        string
	TradeDate     string
	AccDistStatus string
	TotalValue    decimal.Decimal
}
