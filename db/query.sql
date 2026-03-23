-- name: GetOrCreateUser :one
INSERT INTO users (email) VALUES ($1)
ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CreateAuthToken :exec
INSERT INTO auth_tokens (token, user_id, expires_at, client_id) VALUES ($1, $2, $3, $4);

-- name: CreatePendingSession :exec
INSERT INTO pending_sessions (client_id, session_value, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (client_id) DO UPDATE SET session_value = EXCLUDED.session_value, expires_at = EXCLUDED.expires_at;

-- name: GetPendingSession :one
SELECT client_id, session_value, expires_at FROM pending_sessions
WHERE client_id = $1 AND expires_at > now();

-- name: DeletePendingSession :exec
DELETE FROM pending_sessions WHERE client_id = $1;

-- name: GetValidAuthToken :one
SELECT * FROM auth_tokens
WHERE token = $1 AND used_at IS NULL AND expires_at > now();

-- name: MarkAuthTokenUsed :exec
UPDATE auth_tokens SET used_at = now() WHERE token = $1;

-- name: ListActiveTasks :many
SELECT * FROM tasks
WHERE completed_at IS NULL AND user_id = $1
ORDER BY position ASC;

-- name: ListCompletedTasks :many
SELECT * FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= CURRENT_DATE
  AND completed_at < CURRENT_DATE + INTERVAL '1 day'
  AND user_id = $1
ORDER BY completed_at DESC;

-- name: GetMaxPosition :one
SELECT COALESCE(MAX(position), 0)::int FROM tasks WHERE completed_at IS NULL AND user_id = $1;

-- name: CreateTask :one
INSERT INTO tasks (title, position, user_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CompleteTask :one
UPDATE tasks
SET completed_at = now()
WHERE id = $1
RETURNING *;

-- name: UncompleteTask :one
UPDATE tasks
SET completed_at = NULL, position = $2
WHERE id = $1
RETURNING *;

-- name: UpdateTaskPosition :exec
UPDATE tasks SET position = $2 WHERE id = $1;

-- name: DeleteTask :exec
DELETE FROM tasks
WHERE id = $1;

-- name: ListCompletedTasksForDate :many
SELECT * FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= $1::timestamp
  AND completed_at < $1::timestamp + INTERVAL '1 day'
  AND user_id = $2
ORDER BY completed_at DESC;

-- name: GetPrevCompletedDate :one
SELECT DATE_TRUNC('day', completed_at)::timestamp as day
FROM tasks
WHERE completed_at IS NOT NULL
  AND DATE_TRUNC('day', completed_at) < $1::timestamp
  AND user_id = $2
ORDER BY completed_at DESC
LIMIT 1;

-- name: ListHistoricalCompletedTasks :many
SELECT * FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= $1::timestamp
  AND completed_at < $2::timestamp
  AND user_id = $3
ORDER BY completed_at DESC;
