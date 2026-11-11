-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;


-- name: ListKilns :many
-- Active kilns first; inactive ones are kept so old firings still show their kiln.
SELECT * FROM kilns ORDER BY active DESC, id;

-- name: GetKiln :one
SELECT * FROM kilns WHERE id = $1;
