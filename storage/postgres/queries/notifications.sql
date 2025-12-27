-- name: CreateNotification :one
INSERT INTO notifications (user_id, type, message)
VALUES (sqlc.arg('user_id'), sqlc.arg('type'), sqlc.arg('message'))
RETURNING id, user_id, type, message, seen, created_at;

-- name: GetNotificationsByUser :many
SELECT id, user_id, type, message, seen, created_at
FROM notifications
WHERE user_id = sqlc.arg('user_id') AND deleted_at = 0;

-- name: GetUnreadNotificationsByUser :many
SELECT id, user_id, type, message, seen, created_at
FROM notifications
WHERE user_id = sqlc.arg('user_id') AND seen = false AND deleted_at = 0;

-- name: MarkNotificationAsRead :exec
UPDATE notifications 
SET seen = true, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');

-- name: DeleteNotification :exec
DELETE FROM notifications WHERE id = sqlc.arg('id');
