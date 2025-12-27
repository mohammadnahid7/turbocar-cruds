-- name: CreateCar :one
INSERT INTO cars (
    "type",
    "make",
    "model",
    "year",
    "color",
    "mileage",
    "price",
    "description",
    "owner_id",
    "location"
) VALUES (
    sqlc.arg('type'),
    sqlc.arg('make'),
    sqlc.arg('model'),
    sqlc.arg('year'),
    sqlc.arg('color'),
    sqlc.arg('mileage'),
    sqlc.arg('price'),
    sqlc.arg('description'),
    sqlc.arg('owner_id'),
    sqlc.arg('location')
)
RETURNING 
    id, type, make, model, year, color, mileage, price, description, available, 
    owner_id, location, reviews_count, created_at, updated_at;

-- name: GetCarById :one
SELECT 
    c.id, c.type, c.make, c.model, c.year, c.color, c.mileage, c.price, 
    c.description, c.available, c.owner_id, c.location, c.reviews_count, 
    c.created_at, c.updated_at,
    COALESCE(
        json_agg(
            jsonb_build_object(
                'id', i.id,
                'car_id', i.car_id,
                'filename', i.filename,
                'uploaded_at', i.uploaded_at
            )
        ) FILTER (WHERE i.deleted_at = 0), '[]'
    ) AS images
FROM cars c
LEFT JOIN images i ON c.id = i.car_id
WHERE c.id = sqlc.arg('id')
GROUP BY c.id;

-- name: ListCars :many
SELECT 
    c.id, c.type, c.make, c.model, c.year, c.color, c.mileage, c.price, 
    c.description, c.available, c.owner_id, c.location, c.reviews_count, 
    c.created_at, c.updated_at,
    COALESCE(
        json_agg(
            jsonb_build_object(
                'id', i.id,
                'car_id', i.car_id,
                'filename', i.filename,
                'uploaded_at', i.uploaded_at
            )
        ) FILTER (WHERE i.deleted_at = 0), '[]'
    ) AS images
FROM cars c
LEFT JOIN images i ON c.id = i.car_id
WHERE 
    (sqlc.arg('type')::TEXT IS NULL OR c.type = sqlc.arg('type')::TEXT)
    AND (sqlc.arg('location')::TEXT IS NULL OR c.location = sqlc.arg('location')::TEXT)
    -- Add price range filters
    AND (sqlc.arg('min_price')::DECIMAL(10,2) IS NULL OR c.price >= sqlc.arg('min_price')::DECIMAL(10,2))
    AND (sqlc.arg('max_price')::DECIMAL(10,2) IS NULL OR c.price <= sqlc.arg('max_price')::DECIMAL(10,2))
    AND (sqlc.arg('user_id')::UUID IS NULL OR c.owner_id = sqlc.arg('user_id')::UUID)
GROUP BY 
    c.id, c.type, c.make, c.model, c.year, c.color, c.mileage, c.price, 
    c.description, c.available, c.owner_id, c.location, c.reviews_count, 
    c.created_at, c.updated_at
ORDER BY 
    CASE 
        WHEN sqlc.arg('price_order')::TEXT = 'asc' THEN c.price 
    END ASC,
    CASE 
        WHEN sqlc.arg('price_order')::TEXT = 'desc' THEN c.price 
    END DESC,
    c.created_at DESC
LIMIT sqlc.arg('limit')::INTEGER OFFSET sqlc.arg('offset')::INTEGER;


-- name: UpdateCar :exec
UPDATE cars
SET type = sqlc.arg('type'), make = sqlc.arg('make'), model = sqlc.arg('model'), year = sqlc.arg('year'), 
    color = sqlc.arg('color'), mileage = sqlc.arg('mileage'), price = sqlc.arg('price'),
    description = sqlc.arg('description'), available = sqlc.arg('available'), location = sqlc.arg('location'), 
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');

-- name: DeleteCar :exec
DELETE FROM cars WHERE id = sqlc.arg('id');

-- name: IncrementCarReviewCount :exec
UPDATE cars
SET reviews_count = reviews_count + 1
WHERE id = sqlc.arg('id');

-- name: CheckCarOwnership :one
SELECT EXISTS (
    SELECT 1 FROM cars 
    WHERE id = sqlc.arg('id') AND owner_id = sqlc.arg('owner_id')
) AS is_owner;


-- name: SearchCar :many
SELECT 
    c.id, c.type, c.make, c.model, c.year, c.color, c.mileage, c.price, 
    c.description, c.available, c.owner_id, c.location, c.reviews_count, 
    c.created_at, c.updated_at,
    COALESCE(
        json_agg(
            jsonb_build_object(
                'id', i.id,
                'car_id', i.car_id,
                'filename', i.filename,
                'uploaded_at', i.uploaded_at
            )
        ) FILTER (WHERE i.deleted_at = 0), '[]'
    ) AS images
FROM cars c
LEFT JOIN images i ON c.id = i.car_id
WHERE 
    COALESCE(sqlc.arg('query'), '') = '' OR 
    c.make ILIKE '%' || sqlc.arg('query') || '%' OR 
    c.model ILIKE '%' || sqlc.arg('query') || '%'
GROUP BY 
    c.id, c.type, c.make, c.model, c.year, c.color, c.mileage, c.price, 
    c.description, c.available, c.owner_id, c.location, c.reviews_count, 
    c.created_at, c.updated_at
ORDER BY c.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
