CREATE TABLE market_detector_summaries (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    symbol VARCHAR(10) NOT NULL,
    accdist_status VARCHAR(50) NOT NULL,
    total_value NUMERIC(20, 2) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    trade_date DATE NOT NULL,
    UNIQUE (symbol, trade_date)
);

CREATE TABLE broker_transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    summary_id UUID NOT NULL REFERENCES market_detector_summaries (
        id
    ) ON DELETE CASCADE,
    symbol VARCHAR(10) NOT NULL,
    investor_type VARCHAR(20) NOT NULL,
    side VARCHAR(4) CHECK (side IN ('BUY', 'SELL')) NOT NULL,
    broker_code CHAR(2) NOT NULL,
    frequency INTEGER NOT NULL,
    lots BIGINT NOT NULL,
    avg_price NUMERIC(18, 4) NOT NULL,
    trade_date DATE NOT NULL,
    UNIQUE (symbol, trade_date, broker_code, side)
);
