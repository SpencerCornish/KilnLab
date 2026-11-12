-- name: ListUsers :many
SELECT u.*,
       EXISTS (SELECT 1 FROM accounts a WHERE a.user_id = u.id AND a.is_kiln_master) AS is_kiln_master,
       (SELECT a.provider FROM accounts a WHERE a.user_id = u.id ORDER BY a.id LIMIT 1)::auth_provider AS provider
FROM users u
ORDER BY lower(u.display_name), u.id;

-- name: SetUserAdmin :one
UPDATE users SET is_admin = $2 WHERE id = $1 RETURNING *;
