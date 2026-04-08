CREATE TABLE market_detector_summaries (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    symbol VARCHAR(10) NOT NULL,
    accdist_status VARCHAR(50) not null,
    total_value NUMERIC(20, 2) not null,
    created_at TIMESTAMPTZ DEFAULT NOW() not null,
    trade_date DATE NOT NULL,
    UNIQUE(symbol, trade_date)
);

CREATE TABLE broker_transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    summary_id UUID REFERENCES market_detector_summaries(id) ON DELETE CASCADE,
    symbol VARCHAR(10) NOT NULL,
    investor_type VARCHAR(20) not null,
    side VARCHAR(4) CHECK (side IN ('BUY', 'SELL')) not null,
    broker_code CHAR(2) NOT NULL,
    frequency INTEGER not null,
    lots BIGINT not null,
    avg_price NUMERIC(18, 4) not null,
    trade_date DATE NOT NULL,
    UNIQUE(symbol, trade_date, broker_code, side)
);
