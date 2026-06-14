-- name: CreateOrder :one
INSERT INTO orders (
    id, user_id, symbol, side, type,
    price, quantity, filled_qty, status,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11
) RETURNING *;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: CancelOrder :one
UPDATE orders
SET status = 'CANCELED', updated_at = $2
WHERE id = $1
RETURNING *;

-- name: GetOpenOrdersBySymbol :many
SELECT * FROM orders
WHERE symbol = $1 AND status IN ('NEW', 'PARTIAL')
ORDER BY created_at ASC;

-- name: GetAllOpenOrders :many
SELECT * FROM orders
WHERE status IN ('NEW', 'PARTIAL')
ORDER BY created_at ASC;
