-- name: CreateSavedCar :one
INSERT INTO saved_cars (user_id, car_id)
VALUES (
    sqlc.arg('user_id'),
    sqlc.arg('car_id')
)
RETURNING id, user_id, car_id, created_at, updated_at;

-- name: GetSavedCarsByUser :many
SELECT id, user_id, car_id, created_at, updated_at
FROM saved_cars
WHERE user_id = sqlc.arg('user_id');

-- name: DeleteSavedCar :exec
DELETE FROM saved_cars WHERE id = sqlc.arg('id');

-- name: DeleteSavedCarsByCarId :exec
DELETE FROM saved_cars WHERE car_id = sqlc.arg('car_id');

-- name: CheckSavedCarOwnership :one
SELECT EXISTS (
    SELECT 1 FROM saved_cars
    WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
) AS is_owner;
