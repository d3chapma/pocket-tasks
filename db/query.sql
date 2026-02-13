-- name: ListTasks :many
SELECT * FROM tasks
ORDER BY created_at DESC;

-- name: CreateTask :one
INSERT INTO tasks (title)
VALUES ($1)
RETURNING *;

-- name: ToggleTask :one
UPDATE tasks
SET completed = NOT completed
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks
WHERE id = $1;

