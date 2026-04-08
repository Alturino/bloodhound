package models

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestDecimalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected string // use string representation for comparison
		err      bool
	}{
		{
			name:     "regular float in string",
			jsonStr:  `{"val": "2629.611"}`,
			expected: "2629.611",
		},
		{
			name:     "scientific notation string",
			jsonStr:  `{"val": "1.571719e+09"}`,
			expected: "1571719000",
		},
		{
			name:     "actual float type",
			jsonStr:  `{"val": 1571719000.0}`,
			expected: "1571719000",
		},
		{
			name:    "invalid format",
			jsonStr: `{"val": "not-a-number"}`,
			err:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper struct {
				Val decimal.Decimal `json:"val"`
			}
			err := json.Unmarshal([]byte(tt.jsonStr), &wrapper)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, wrapper.Val.String())
			}
		})
	}
}

func TestStockbitBrokerTransactionJSON(t *testing.T) {
	jsonStr := `{
		"net_broker_code": "AZ",
		"netbs_stock_code": "DAAZ",
		"netbs_date": "2026-03-13",
		"type": "FIXED",
		"netbs_buy_avg_price": "2629.611",
		"netbs_sell_avg_price": "0",
		"blot": "5977",
		"slot": "0",
		"freq": "123"
	}`

	var trans StockbitBrokerTransaction
	err := json.Unmarshal([]byte(jsonStr), &trans)
	assert.NoError(t, err)

	assert.Equal(t, "AZ", trans.BrokerCode)
	assert.Equal(t, "DAAZ", trans.StockCode)
	assert.Equal(t, "2026-03-13", trans.Date)
	assert.True(t, trans.BuyAvgPrice.Equal(decimal.NewFromFloat(2629.611)))
	assert.True(t, trans.BLot.Equal(decimal.NewFromInt(5977)))
	assert.Equal(t, int64(123), trans.Freq)
}
