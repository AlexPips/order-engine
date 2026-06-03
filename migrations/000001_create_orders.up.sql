CREATE TABLE IF NOT EXISTS orders (
    id        TEXT PRIMARY KEY,
    user_id   TEXT NOT NULL,
    symbol    TEXT NOT NULL,
    side      TEXT NOT NULL CHECK (side IN ('BUY', 'SELL')),
    type      TEXT NOT NULL CHECK (type IN ('LIMIT', 'MARKET')),
    price     NUMERIC(20,8) NOT NULL,
    quantity  NUMERIC(20,8) NOT NULL,
    filled_qty NUMERIC(20,8) NOT NULL DEFAULT 0,
    status    TEXT NOT NULL CHECK (status IN ('NEW', 'PARTIAL', 'FILLED', 'CANCELED', 'REJECTED')),
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

CREATE INDEX idx_orders_symbol_created ON orders (symbol, created_at);
CREATE INDEX idx_orders_user_id ON orders (user_id);