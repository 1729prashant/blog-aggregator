-- name: AddFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, last_fetched_at, user_id)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
)
RETURNING *;
--


-- name: GetFeed :one
SELECT name FROM feeds WHERE name = $1 AND user_id = $2 LIMIT 1;
--


-- name: GetAllFeeds :many
SELECT f.name, f.url, u.name
FROM feeds f, users u
where u.id = f.user_id
ORDER BY f.name;
--


-- name: GetFeedNamebyURL :one
SELECT name, id FROM feeds WHERE url = $1 LIMIT 1;
--



-- name: MarkFeedFetched :exec
UPDATE feeds set last_fetched_at = $1, updated_at = $2
WHERE id = $3;
--