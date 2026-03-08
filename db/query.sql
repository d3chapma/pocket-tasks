-- name: ListActiveTasks :many
SELECT * FROM tasks
WHERE completed_at IS NULL
ORDER BY position ASC;

-- name: ListCompletedTasks :many
SELECT * FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= CURRENT_DATE
  AND completed_at < CURRENT_DATE + INTERVAL '1 day'
ORDER BY completed_at DESC;

-- name: GetMaxPosition :one
SELECT COALESCE(MAX(position), 0)::int FROM tasks WHERE completed_at IS NULL;

-- name: CreateTask :one
INSERT INTO tasks (title, position)
VALUES ($1, $2)
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
ORDER BY completed_at DESC;

-- name: GetPrevCompletedDate :one
SELECT DATE_TRUNC('day', completed_at)::timestamp as day
FROM tasks
WHERE completed_at IS NOT NULL
  AND DATE_TRUNC('day', completed_at) < $1::timestamp
ORDER BY completed_at DESC
LIMIT 1;

-- name: ListHistoricalCompletedTasks :many
SELECT * FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= $1::timestamp
  AND completed_at < $2::timestamp
ORDER BY completed_at DESC;
