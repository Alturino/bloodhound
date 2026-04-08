package models

import (
	"github.com/shopspring/decimal"
)

// StockbitMarketDetectorResponse represents the JSON response from Stockbit
type StockbitMarketDetectorResponse struct {
	Message string             `json:"message"`
	Data    StockbitMarketData `json:"data"`
}

type StockbitMarketData struct {
	From           string                 `json:"from"`
	To             string                 `json:"to"`
	BandarDetector StockbitBandarDetector `json:"bandar_detector"`
	BrokerSummary  StockbitBrokerSummary  `json:"broker_summary"`
}

type StockbitBandarDetector struct {
	NumberBrokerBuySell int                  `json:"number_broker_buysell"`
	TotalBuyer          int                  `json:"total_buyer"`
	TotalSeller         int                  `json:"total_seller"`
	Value               decimal.Decimal      `json:"value"`
	Volume              decimal.Decimal      `json:"volume"`
	Average             decimal.Decimal      `json:"average"`
	BrokerAccDist       string               `json:"broker_accdist"`
	Top1                StockbitAccDistLevel `json:"top1"`
	Top3                StockbitAccDistLevel `json:"top3"`
	Top5                StockbitAccDistLevel `json:"top5"`
	Top10               StockbitAccDistLevel `json:"top10"`
	Avg                 StockbitAccDistLevel `json:"avg"`
	Avg5                StockbitAccDistLevel `json:"avg5"`
}

type StockbitAccDistLevel struct {
	AccDist string          `json:"accdist"`
	Amount  decimal.Decimal `json:"amount"`
	Percent decimal.Decimal `json:"percent"`
	Vol     decimal.Decimal `json:"vol"`
}

type StockbitBrokerSummary struct {
	Symbol      string                      `json:"symbol"`
	BrokersBuy  []StockbitBrokerTransaction `json:"brokers_buy"`
	BrokersSell []StockbitBrokerTransaction `json:"brokers_sell"`
}

type StockbitBrokerTransaction struct {
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

type BrokerTransaction struct {
	Lots         int64
	Frequency    int64
	Symbol       string
	TradeDate    string
	BrokerCode   string
	Side         string
	InvestorType string
	AvgPrice     decimal.Decimal
}
