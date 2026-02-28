-- name: ListActiveTasks :many
SELECT * FROM tasks
WHERE completed_at IS NULL
ORDER BY created_at DESC;

-- name: ListCompletedTasks :many
SELECT * FROM tasks
WHERE completed_at IS NOT NULL
ORDER BY completed_at DESC;

-- name: CreateTask :one
INSERT INTO tasks (title)
VALUES ($1)
RETURNING *;

-- name: CompleteTask :one
UPDATE tasks
SET completed_at = now()
WHERE id = $1
RETURNING *;

-- name: UncompleteTask :one
UPDATE tasks
SET completed_at = NULL
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks
WHERE id = $1;
