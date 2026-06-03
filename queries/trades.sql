-- name: CreateTrade :one
INSERT INTO trades (
    id, symbol, buy_order_id, sell_order_id,
    price, quantity, executed_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7
) RETURNING *;

-- name: GetTradesBySymbol :many
SELECT * FROM trades
WHERE symbol = $1
ORDER BY executed_at DESC;

-- name: GetTradesByOrder :many
SELECT * FROM trades
WHERE buy_order_id = $1 OR sell_order_id = $1
ORDER BY executed_at DESC;
