package stockbit

import "github.com/shopspring/decimal"

type BrokerActivityResponse struct {
	Message string             `json:"message"`
	Data    BrokerActivityData `json:"data"`
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
