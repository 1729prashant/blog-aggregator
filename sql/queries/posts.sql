-- name: CreatePost :one
INSERT INTO posts (
    id,
    created_at,
    updated_at,
    title,
    url,
    description,
    published_at,
    feed_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;
--


-- name: GetPostsForUser :many
SELECT f.name, p.title, p.url , p.description, p.published_at
FROM posts p
JOIN feeds f ON p.feed_id = f.id
JOIN feed_follows ff ON f.id = ff.feed_id
WHERE ff.user_id = (
    SELECT u.id FROM users u WHERE u.name = $1
)
ORDER BY p.published_at DESC
LIMIT $2;
--