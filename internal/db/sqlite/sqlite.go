package sqlite

import (
	"context"
	"database/sql"
	"time"

	db "github.com/d3chapma/pocket-tasks/internal/db"
)

const timeFormat = "2006-01-02 15:04:05"

type Queries struct {
	db *sql.DB
}

func New(d *sql.DB) *Queries {
	return &Queries{db: d}
}

var _ db.Querier = (*Queries)(nil)

func parseNullTime(s sql.NullString) sql.NullTime {
	if !s.Valid {
		return sql.NullTime{}
	}
	t, err := time.Parse(timeFormat, s.String)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func scanTask(row interface {
	Scan(...any) error
}) (db.Task, error) {
	var t db.Task
	var completedAt sql.NullString
	var createdAt string
	err := row.Scan(&t.ID, &t.Title, &completedAt, &t.Position, &createdAt)
	if err != nil {
		return t, err
	}
	t.CompletedAt = parseNullTime(completedAt)
	t.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	return t, nil
}

const completeTask = `
UPDATE tasks
SET completed_at = strftime('%Y-%m-%d %H:%M:%S', 'now')
WHERE id = ?
RETURNING id, title, completed_at, position, created_at
`

func (q *Queries) CompleteTask(ctx context.Context, id int32) (db.Task, error) {
	return scanTask(q.db.QueryRowContext(ctx, completeTask, id))
}

const createTask = `
INSERT INTO tasks (title, position)
VALUES (?, ?)
RETURNING id, title, completed_at, position, created_at
`

func (q *Queries) CreateTask(ctx context.Context, arg db.CreateTaskParams) (db.Task, error) {
	return scanTask(q.db.QueryRowContext(ctx, createTask, arg.Title, arg.Position))
}

const deleteTask = `DELETE FROM tasks WHERE id = ?`

func (q *Queries) DeleteTask(ctx context.Context, id int32) error {
	_, err := q.db.ExecContext(ctx, deleteTask, id)
	return err
}

const getMaxPosition = `SELECT COALESCE(MAX(position), 0) FROM tasks WHERE completed_at IS NULL`

func (q *Queries) GetMaxPosition(ctx context.Context) (int32, error) {
	row := q.db.QueryRowContext(ctx, getMaxPosition)
	var v int32
	err := row.Scan(&v)
	return v, err
}

const getPrevCompletedDate = `
SELECT strftime('%Y-%m-%d 00:00:00', completed_at) as day
FROM tasks
WHERE completed_at IS NOT NULL
  AND strftime('%Y-%m-%d', completed_at) < strftime('%Y-%m-%d', ?)
ORDER BY completed_at DESC
LIMIT 1
`

func (q *Queries) GetPrevCompletedDate(ctx context.Context, before time.Time) (time.Time, error) {
	row := q.db.QueryRowContext(ctx, getPrevCompletedDate, before.UTC().Format(timeFormat))
	var dayStr string
	if err := row.Scan(&dayStr); err != nil {
		return time.Time{}, err
	}
	return time.Parse(timeFormat, dayStr)
}

const listActiveTasks = `
SELECT id, title, completed_at, position, created_at FROM tasks
WHERE completed_at IS NULL
ORDER BY position ASC
`

func (q *Queries) ListActiveTasks(ctx context.Context) ([]db.Task, error) {
	return scanTasks(q.db.QueryContext(ctx, listActiveTasks))
}

const listCompletedTasks = `
SELECT id, title, completed_at, position, created_at FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= strftime('%Y-%m-%d 00:00:00', 'now')
  AND completed_at < strftime('%Y-%m-%d 00:00:00', 'now', '+1 day')
ORDER BY completed_at DESC
`

func (q *Queries) ListCompletedTasks(ctx context.Context) ([]db.Task, error) {
	return scanTasks(q.db.QueryContext(ctx, listCompletedTasks))
}

const listCompletedTasksForDate = `
SELECT id, title, completed_at, position, created_at FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= strftime('%Y-%m-%d 00:00:00', ?)
  AND completed_at < strftime('%Y-%m-%d 00:00:00', ?, '+1 day')
ORDER BY completed_at DESC
`

func (q *Queries) ListCompletedTasksForDate(ctx context.Context, t time.Time) ([]db.Task, error) {
	s := t.UTC().Format(timeFormat)
	return scanTasks(q.db.QueryContext(ctx, listCompletedTasksForDate, s, s))
}

const listHistoricalCompletedTasks = `
SELECT id, title, completed_at, position, created_at FROM tasks
WHERE completed_at IS NOT NULL
  AND completed_at >= ?
  AND completed_at < ?
ORDER BY completed_at DESC
`

func (q *Queries) ListHistoricalCompletedTasks(ctx context.Context, arg db.ListHistoricalCompletedTasksParams) ([]db.Task, error) {
	return scanTasks(q.db.QueryContext(ctx, listHistoricalCompletedTasks,
		arg.Column1.UTC().Format(timeFormat),
		arg.Column2.UTC().Format(timeFormat),
	))
}

const uncompleteTask = `
UPDATE tasks
SET completed_at = NULL, position = ?
WHERE id = ?
RETURNING id, title, completed_at, position, created_at
`

func (q *Queries) UncompleteTask(ctx context.Context, arg db.UncompleteTaskParams) (db.Task, error) {
	return scanTask(q.db.QueryRowContext(ctx, uncompleteTask, arg.Position, arg.ID))
}

const updateTaskPosition = `UPDATE tasks SET position = ? WHERE id = ?`

func (q *Queries) UpdateTaskPosition(ctx context.Context, arg db.UpdateTaskPositionParams) error {
	_, err := q.db.ExecContext(ctx, updateTaskPosition, arg.Position, arg.ID)
	return err
}

func scanTasks(rows *sql.Rows, err error) ([]db.Task, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []db.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}
