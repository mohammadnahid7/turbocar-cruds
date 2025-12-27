-- name: CreateMessage :one
INSERT INTO messages (sender_id, recipient_id, content)
VALUES (sqlc.arg('sender_id'), sqlc.arg('recipient_id'), sqlc.arg('content'))
RETURNING id, sender_id, recipient_id, content, read, created_at, updated_at;

-- name: GetMessagesByUser :many
SELECT 
    CASE 
        WHEN sender_id = sqlc.arg('user_id') THEN recipient_id
        ELSE sender_id
    END AS user_id,
    json_agg(
        json_build_object(
            'id', id,
            'sender_id', sender_id,
            'recipient_id', recipient_id,
            'content', content,
            'read', read,
            'created_at', created_at,
            'updated_at', updated_at
        ) ORDER BY created_at ASC
    ) AS messages
FROM messages
WHERE sender_id = sqlc.arg('user_id') OR recipient_id = sqlc.arg('user_id')
GROUP BY user_id;

-- name: MarkMessageAsRead :exec
UPDATE messages
SET read = true, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = sqlc.arg('id');

-- name: CheckMessageOwnership :one
SELECT EXISTS (
    SELECT 1 FROM messages 
    WHERE id = sqlc.arg('id') AND (sender_id = sqlc.arg('user_id') OR recipient_id = sqlc.arg('user_id'))
) AS is_owner;

-- name: GetMessagesByUserAndId :many
SELECT 
    id,
    sender_id,
    recipient_id,
    content,
    read,
    created_at,
    updated_at
FROM messages
WHERE 
    (sender_id = sqlc.arg('user1_id') AND recipient_id = sqlc.arg('user2_id'))
    OR 
    (sender_id = sqlc.arg('user2_id') AND recipient_id = sqlc.arg('user1_id'))
ORDER BY created_at ASC;
