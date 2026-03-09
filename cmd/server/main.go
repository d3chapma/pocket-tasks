package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/d3chapma/pocket-tasks/db/migrations"
	sqlitemigrations "github.com/d3chapma/pocket-tasks/db/migrations/sqlite"
	"github.com/d3chapma/pocket-tasks/internal/db"
	sqlitedb "github.com/d3chapma/pocket-tasks/internal/db/sqlite"
	"github.com/d3chapma/pocket-tasks/internal/views"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func prevDateFor(ctx context.Context, queries db.Querier, day time.Time) string {
	if _, err := queries.GetPrevCompletedDate(ctx, day); err == nil {
		return day.Format("2006-01-02")
	}
	return ""
}

func groupTasksByDay(tasks []db.Task) []views.CompletedDay {
	var days []views.CompletedDay
	var curDay time.Time
	var curTasks []db.Task
	for _, t := range tasks {
		if !t.CompletedAt.Valid {
			continue
		}
		day := startOfDay(t.CompletedAt.Time)
		if !day.Equal(curDay) {
			if len(curTasks) > 0 {
				days = append(days, views.CompletedDay{Label: curDay.Format("Mon Jan 2"), Tasks: curTasks})
			}
			curDay = day
			curTasks = []db.Task{t}
		} else {
			curTasks = append(curTasks, t)
		}
	}
	if len(curTasks) > 0 {
		days = append(days, views.CompletedDay{Label: curDay.Format("Mon Jan 2"), Tasks: curTasks})
	}
	return days
}

// loadHistoryDays re-fetches history for the given oldest date parameter.
// Returns the history days and the prevDate for the "load more" button.
func loadHistoryDays(ctx context.Context, queries db.Querier, historyOldest string) ([]views.CompletedDay, string) {
	if historyOldest == "" {
		return nil, ""
	}
	oldest, err := time.Parse("2006-01-02", historyOldest)
	if err != nil {
		return nil, ""
	}
	oldestDay := startOfDay(oldest)
	prevRow, err := queries.GetPrevCompletedDate(ctx, oldestDay)
	if err != nil {
		return nil, ""
	}
	newOldest := startOfDay(prevRow)
	today := startOfDay(time.Now().UTC())
	historical, _ := queries.ListHistoricalCompletedTasks(ctx, db.ListHistoricalCompletedTasksParams{
		Column1: newOldest,
		Column2: today,
	})
	return groupTasksByDay(historical), prevDateFor(ctx, queries, newOldest)
}

func renderTasks(w http.ResponseWriter, r *http.Request, queries db.Querier, selectedIndex int) {
	ctx := r.Context()
	active, _ := queries.ListActiveTasks(ctx)
	completed, _ := queries.ListCompletedTasks(ctx)

	total := len(active) + len(completed)
	if selectedIndex >= total {
		selectedIndex = total - 1
	}
	if selectedIndex < 0 {
		selectedIndex = 0
	}

	today := startOfDay(time.Now().UTC())
	historyOldest := r.URL.Query().Get("historyOldest")
	historyDays, histPrevDate := loadHistoryDays(ctx, queries, historyOldest)
	prevDate := prevDateFor(ctx, queries, today)
	if historyOldest != "" {
		prevDate = histPrevDate
	}

	_ = views.TaskList(active, completed, selectedIndex, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
}

func openDB(ctx context.Context, databaseURL string) (*sql.DB, db.Querier, func(), error) {
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			return nil, nil, nil, err
		}
		sqlDB := stdlib.OpenDBFromPool(pool)
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			pool.Close()
			sqlDB.Close()
			return nil, nil, nil, err
		}
		if err := goose.Up(sqlDB, "."); err != nil {
			pool.Close()
			sqlDB.Close()
			return nil, nil, nil, err
		}
		queries := db.New(sqlDB)
		cleanup := func() {
			sqlDB.Close()
			pool.Close()
		}
		return sqlDB, queries, cleanup, nil
	}

	sqlDB, err := sql.Open("sqlite", databaseURL)
	if err != nil {
		return nil, nil, nil, err
	}
	goose.SetBaseFS(sqlitemigrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		sqlDB.Close()
		return nil, nil, nil, err
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		sqlDB.Close()
		return nil, nil, nil, err
	}
	queries := sqlitedb.New(sqlDB)
	return sqlDB, queries, func() { sqlDB.Close() }, nil
}

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	_, queries, cleanup, err := openDB(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		active, err := queries.ListActiveTasks(r.Context())
		if err != nil {
			log.Println("ListActiveTasks error:", err)
			http.Error(w, "Failed to load tasks", 500)
			return
		}

		completed, err := queries.ListCompletedTasks(r.Context())
		if err != nil {
			log.Println("ListCompletedTasks error:", err)
			http.Error(w, "Failed to load tasks", 500)
			return
		}

		today := startOfDay(time.Now().UTC())
		_ = views.Layout(
			views.TaskList(active, completed, 0, "Completed Today", prevDateFor(r.Context(), queries, today), nil, ""),
		).Render(r.Context(), w)
	})

	r.Post("/tasks", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid Form", 400)
			return
		}

		title := r.FormValue("title")
		selectedIndex, _ := strconv.Atoi(r.URL.Query().Get("selectedIndex"))
		historyOldest := r.URL.Query().Get("historyOldest")

		active, _ := queries.ListActiveTasks(r.Context())
		_, _ = queries.ListCompletedTasks(r.Context())

		// Determine insert position
		var insertAfterIdx int
		if selectedIndex < len(active) {
			insertAfterIdx = selectedIndex
		} else {
			insertAfterIdx = len(active) - 1
		}

		// Calculate the new task's position value
		var newPosition int32
		if len(active) == 0 {
			newPosition = 1
		} else if insertAfterIdx == len(active)-1 {
			// Insert at the end
			newPosition = active[insertAfterIdx].Position + 1
		} else {
			// Insert between two tasks - shift everything below down
			newPosition = active[insertAfterIdx].Position + 1
			for i := insertAfterIdx + 1; i < len(active); i++ {
				_ = queries.UpdateTaskPosition(r.Context(), db.UpdateTaskPositionParams{
					ID:       active[i].ID,
					Position: active[i].Position + 1,
				})
			}
		}

		_, err := queries.CreateTask(r.Context(), db.CreateTaskParams{
			Title:    title,
			Position: newPosition,
		})
		if err != nil {
			http.Error(w, "Failed to create task", 500)
			return
		}

		active, _ = queries.ListActiveTasks(r.Context())
		completed, _ := queries.ListCompletedTasks(r.Context())

		// Select the newly created task (it's right after insertAfterIdx)
		newSelectedIndex := insertAfterIdx + 1
		if len(active) == 1 {
			newSelectedIndex = 0
		}

		ctx := r.Context()
		today := startOfDay(time.Now().UTC())
		historyDays, histPrevDate := loadHistoryDays(ctx, queries, historyOldest)
		prevDate := prevDateFor(ctx, queries, today)
		if historyOldest != "" {
			prevDate = histPrevDate
		}

		_ = views.TaskList(active, completed, newSelectedIndex, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
	})

	r.Post("/tasks/complete/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}
		historyOldest := r.URL.Query().Get("historyOldest")

		_, err = queries.CompleteTask(r.Context(), int32(id))
		if err != nil {
			http.Error(w, "Failed to complete task", 500)
			return
		}

		ctx := r.Context()
		active, _ := queries.ListActiveTasks(ctx)
		completed, _ := queries.ListCompletedTasks(ctx)

		// Task is now first in completed list, index = len(active)
		newIndex := len(active)
		total := len(active) + len(completed)
		if newIndex >= total {
			newIndex = total - 1
		}

		today := startOfDay(time.Now().UTC())
		historyDays, histPrevDate := loadHistoryDays(ctx, queries, historyOldest)
		prevDate := prevDateFor(ctx, queries, today)
		if historyOldest != "" {
			prevDate = histPrevDate
		}

		_ = views.TaskList(active, completed, newIndex, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
	})

	r.Post("/tasks/uncomplete/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}
		historyOldest := r.URL.Query().Get("historyOldest")

		maxPos, _ := queries.GetMaxPosition(r.Context())

		_, err = queries.UncompleteTask(r.Context(), db.UncompleteTaskParams{
			ID:       int32(id),
			Position: maxPos + 1,
		})
		if err != nil {
			http.Error(w, "Failed to uncomplete task", 500)
			return
		}

		ctx := r.Context()
		active, _ := queries.ListActiveTasks(ctx)
		completed, _ := queries.ListCompletedTasks(ctx)

		// Task is now last in active list
		newIndex := len(active) - 1
		if newIndex < 0 {
			newIndex = 0
		}

		today := startOfDay(time.Now().UTC())
		historyDays, histPrevDate := loadHistoryDays(ctx, queries, historyOldest)
		prevDate := prevDateFor(ctx, queries, today)
		if historyOldest != "" {
			prevDate = histPrevDate
		}

		_ = views.TaskList(active, completed, newIndex, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
	})

	r.Post("/tasks/move/{id}/{direction}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}
		direction := chi.URLParam(r, "direction")

		active, _ := queries.ListActiveTasks(r.Context())

		// Find task index
		idx := -1
		for i, t := range active {
			if t.ID == int32(id) {
				idx = i
				break
			}
		}
		if idx == -1 {
			http.Error(w, "Task not found", 404)
			return
		}

		var newIdx int
		switch direction {
		case "up":
			if idx == 0 {
				renderTasks(w, r, queries, idx)
				return
			}
			newIdx = idx - 1
		case "down":
			if idx == len(active)-1 {
				renderTasks(w, r, queries, idx)
				return
			}
			newIdx = idx + 1
		case "top":
			newIdx = 0
		case "bottom":
			newIdx = len(active) - 1
		default:
			http.Error(w, "Invalid direction", 400)
			return
		}

		// Reorder the slice
		task := active[idx]
		active = slices.Delete(active, idx, idx+1)
		active = slices.Insert(active, newIdx, task)

		// Reassign positions
		for i, t := range active {
			_ = queries.UpdateTaskPosition(r.Context(), db.UpdateTaskPositionParams{
				ID:       t.ID,
				Position: int32(i + 1),
			})
		}

		ctx := r.Context()
		historyOldest := r.URL.Query().Get("historyOldest")
		completed, _ := queries.ListCompletedTasks(ctx)
		today := startOfDay(time.Now().UTC())
		historyDays, histPrevDate := loadHistoryDays(ctx, queries, historyOldest)
		prevDate := prevDateFor(ctx, queries, today)
		if historyOldest != "" {
			prevDate = histPrevDate
		}

		_ = views.TaskList(active, completed, newIdx, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
	})

	r.Get("/tasks/history", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		oldestStr := r.URL.Query().Get("oldest")
		oldest, err := time.Parse("2006-01-02", oldestStr)
		if err != nil {
			http.Error(w, "Invalid date", http.StatusBadRequest)
			return
		}

		oldestDay := startOfDay(oldest)
		prevRow, err := queries.GetPrevCompletedDate(ctx, oldestDay)
		if err != nil {
			http.Error(w, "No previous day found", http.StatusNotFound)
			return
		}

		newOldest := startOfDay(prevRow)
		today := startOfDay(time.Now().UTC())

		historical, _ := queries.ListHistoricalCompletedTasks(ctx, db.ListHistoricalCompletedTasksParams{
			Column1: newOldest,
			Column2: today,
		})
		historyDays := groupTasksByDay(historical)

		active, _ := queries.ListActiveTasks(ctx)
		completed, _ := queries.ListCompletedTasks(ctx)

		selectedIndex := 0
		if si := r.URL.Query().Get("selectedIndex"); si != "" {
			if n, err := strconv.Atoi(si); err == nil {
				selectedIndex = n
			}
		}
		total := len(active) + len(completed)
		for _, day := range historyDays {
			total += len(day.Tasks)
		}
		if selectedIndex >= total && total > 0 {
			selectedIndex = total - 1
		}
		if selectedIndex < 0 {
			selectedIndex = 0
		}

		_ = views.TaskList(active, completed, selectedIndex, "Completed Today", prevDateFor(ctx, queries, newOldest), historyDays, oldestStr).Render(ctx, w)
	})

	r.Delete("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}

		selectedIndex, _ := strconv.Atoi(r.URL.Query().Get("selectedIndex"))

		err = queries.DeleteTask(r.Context(), int32(id))
		if err != nil {
			fmt.Println("Failed to delete task:", err)
			http.Error(w, "Failed to delete task", 500)
			return
		}

		renderTasks(w, r, queries, selectedIndex)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Listening on port", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
