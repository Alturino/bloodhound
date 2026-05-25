package stockbit

import "github.com/shopspring/decimal"

type Response[T any] struct {
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// BrokerActivityData contains the broker activity data
type BrokerActivityData struct {
	DateFrom string                 `json:"date_from"`
	DateTo   string                 `json:"date_to"`
	Records  []BrokerActivityRecord `json:"records"`
}

// BrokerActivityRecord represents a single broker activity record
type BrokerActivityRecord struct {
	Date          string        `json:"date"`
	BrokerCode    string        `json:"broker_code"`
	TradeActivity TradeActivity `json:"trade_activity"`
}

// TradeActivity contains buy and sell summary statistics
type TradeActivity struct {
	BuySummary  SummaryStats `json:"buy_summary"`
	SellSummary SummaryStats `json:"sell_summary"`
}

// SummaryStats contains summary statistics for buying or selling
type SummaryStats struct {
	AvgPrice decimal.Decimal `json:"avg_price"`
	Freq     int64           `json:"freq"`
	Lot      int64           `json:"lot"`
	Value    decimal.Decimal `json:"value"`
}

type HistoricalBrokerTransaction struct {
	Lots         int64
	Frequency    int64
	Symbol       string
	TradeDate    string
	BrokerCode   string
	Side         string
	InvestorType string
	AvgPrice     decimal.Decimal
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
	Symbol      string                            `json:"symbol"`
	BrokersBuy  []MarketDetectorBrokerTransaction `json:"brokers_buy"`
	BrokersSell []MarketDetectorBrokerTransaction `json:"brokers_sell"`
}

type MarketDetectorBrokerTransaction struct {
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
