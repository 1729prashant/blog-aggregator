-- name: CreateFeedFollow :one
INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5
)
RETURNING *;
--

-- name: GetFeedFollowsForUser :many
SELECT f.name , u.name
FROM feed_follows ff, feeds f, users u 
WHERE ff.feed_id = f.id 
AND ff.user_id = u.id 
AND u.name = $1;
--


-- name: GetFeedIDUserIDfromFollows :one
SELECT ff.feed_id , ff.user_id
FROM feed_follows ff, feeds f, users u 
WHERE ff.feed_id = f.id 
AND ff.user_id = u.id 
AND u.name = $1
AND f.url = $2;
--

-- name: DeleteFeedFollow :exec
DELETE FROM feed_follows WHERE feed_id = $1 AND user_id = $2;
--