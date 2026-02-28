package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"

	"github.com/d3chapma/pocket-tasks/internal/db"
	"github.com/d3chapma/pocket-tasks/internal/views"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func renderTasks(w http.ResponseWriter, r *http.Request, queries *db.Queries, selectedIndex int) {
	active, _ := queries.ListActiveTasks(r.Context())
	completed, _ := queries.ListCompletedTasks(r.Context())

	total := len(active) + len(completed)
	if selectedIndex >= total {
		selectedIndex = total - 1
	}
	if selectedIndex < 0 {
		selectedIndex = 0
	}

	_ = views.TaskList(active, completed, selectedIndex).Render(r.Context(), w)
}

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	queries := db.New(pool)

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

		_ = views.Layout(
			views.TaskList(active, completed, 0),
		).Render(r.Context(), w)
	})

	r.Post("/tasks", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid Form", 400)
			return
		}

		title := r.FormValue("title")

		maxPos, err := queries.GetMaxPosition(r.Context())
		if err != nil {
			maxPos = 0
		}

		_, err = queries.CreateTask(r.Context(), db.CreateTaskParams{
			Title:    title,
			Position: maxPos + 1,
		})
		if err != nil {
			http.Error(w, "Failed to create task", 500)
			return
		}

		renderTasks(w, r, queries, 0)
	})

	r.Post("/tasks/complete/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}

		_, err = queries.CompleteTask(r.Context(), int32(id))
		if err != nil {
			http.Error(w, "Failed to complete task", 500)
			return
		}

		active, _ := queries.ListActiveTasks(r.Context())
		completed, _ := queries.ListCompletedTasks(r.Context())

		// Task is now first in completed list, index = len(active)
		newIndex := len(active)
		total := len(active) + len(completed)
		if newIndex >= total {
			newIndex = total - 1
		}

		_ = views.TaskList(active, completed, newIndex).Render(r.Context(), w)
	})

	r.Post("/tasks/uncomplete/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}

		maxPos, _ := queries.GetMaxPosition(r.Context())

		_, err = queries.UncompleteTask(r.Context(), db.UncompleteTaskParams{
			ID:       int32(id),
			Position: maxPos + 1,
		})
		if err != nil {
			http.Error(w, "Failed to uncomplete task", 500)
			return
		}

		active, _ := queries.ListActiveTasks(r.Context())
		completed, _ := queries.ListCompletedTasks(r.Context())

		// Task is now last in active list
		newIndex := len(active) - 1
		if newIndex < 0 {
			newIndex = 0
		}

		_ = views.TaskList(active, completed, newIndex).Render(r.Context(), w)
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

		completed, _ := queries.ListCompletedTasks(r.Context())
		_ = views.TaskList(active, completed, newIdx).Render(r.Context(), w)
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
