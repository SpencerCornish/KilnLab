-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- name: GetActiveKiln :one
SELECT * FROM kilns WHERE active ORDER BY id LIMIT 1;
