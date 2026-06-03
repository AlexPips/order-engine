CREATE TABLE IF NOT EXISTS trades (
    id            TEXT PRIMARY KEY,
    symbol        TEXT NOT NULL,
    buy_order_id  TEXT NOT NULL,
    sell_order_id TEXT NOT NULL,
    price         NUMERIC(20,8) NOT NULL,
    quantity      NUMERIC(20,8) NOT NULL,
    executed_at   BIGINT NOT NULL
);

CREATE INDEX idx_trades_symbol ON trades (symbol);
CREATE INDEX idx_trades_buy_order ON trades (buy_order_id);
CREATE INDEX idx_trades_sell_order ON trades (sell_order_id);
