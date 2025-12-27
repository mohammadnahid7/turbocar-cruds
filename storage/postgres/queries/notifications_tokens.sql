-- name: CreateNotificationToken :one
INSERT INTO notifications_tokens (user_id, token, platform)
VALUES (sqlc.arg('user_id'), sqlc.arg('token'), sqlc.arg('platform'))
RETURNING id, user_id, token, platform, created_at, updated_at;

-- name: GetNotificationTokensByUserId :many
SELECT id, user_id, token, platform, created_at, updated_at
FROM notifications_tokens
WHERE user_id = sqlc.arg('user_id');

-- name: UpdateNotificationToken :exec
UPDATE notifications_tokens
SET token = sqlc.arg('token'), platform = sqlc.arg('platform'), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');

-- name: DeleteNotificationToken :exec
DELETE FROM notifications_tokens WHERE id = sqlc.arg('id');

-- name: DeleteNotificationTokensByUserId :exec
DELETE FROM notifications_tokens WHERE user_id = sqlc.arg('user_id');
