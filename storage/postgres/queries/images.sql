-- name: AddImage :one
INSERT INTO images (car_id, filename)
VALUES (sqlc.arg('car_id'), sqlc.arg('filename'))
RETURNING id, car_id, filename, uploaded_at;

-- name: GetImagesByCar :many
SELECT id, car_id, filename, uploaded_at
FROM images
WHERE car_id = sqlc.arg('car_id');

-- name: GetImageById :one
SELECT id, car_id, filename, uploaded_at
FROM images
WHERE id = sqlc.arg('id');

-- name: DeleteImage :exec
DELETE FROM images WHERE id = sqlc.arg('id');

-- name: DeleteImagesByCarId :exec
DELETE FROM images WHERE car_id = sqlc.arg('car_id');
