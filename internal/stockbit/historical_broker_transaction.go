package stockbit

import "github.com/shopspring/decimal"

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
