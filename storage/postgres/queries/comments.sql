-- name: CreateComment :one
INSERT INTO comments (
    "user_id",
    "car_id",
    "content"
) VALUES (
    sqlc.arg('user_id'),
    sqlc.arg('car_id'),
    sqlc.arg('content')
)
RETURNING 
    id, user_id, car_id, content, created_at, updated_at;

-- name: GetCommentsByCar :many
SELECT 
    id, user_id, car_id, content, created_at, updated_at
FROM comments
WHERE car_id = sqlc.arg('car_id') AND deleted_at = 0;

-- name: UpdateComment :exec
UPDATE comments
SET 
    content = sqlc.arg('content'), 
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');

-- name: DeleteComment :exec
DELETE FROM comments 
WHERE id = sqlc.arg('id');

-- name: DeleteCommentsByCarId :exec
DELETE FROM comments 
WHERE car_id = sqlc.arg('car_id');

-- name: CheckCommentOwnership :one
SELECT EXISTS (
    SELECT 1 FROM comments 
    WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
) AS is_owner;
