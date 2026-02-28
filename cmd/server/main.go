package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/d3chapma/pocket-tasks/internal/db"
	"github.com/d3chapma/pocket-tasks/internal/views"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
			http.Error(w, "Failed to load tasks", 500)
			return
		}
		completed, err := queries.ListCompletedTasks(r.Context())
		if err != nil {
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
		_, err := queries.CreateTask(r.Context(), title)
		if err != nil {
			http.Error(w, "Failed to create task", 500)
			return
		}

		active, _ := queries.ListActiveTasks(r.Context())
		completed, _ := queries.ListCompletedTasks(r.Context())
		_ = views.TaskList(active, completed, 0).Render(r.Context(), w)
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

		// Task moved from active to top of completed list
		// New index = len(active) since it's now first in completed
		newIndex := len(active)
		// But if the user was beyond the list, clamp it
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

		_, err = queries.UncompleteTask(r.Context(), int32(id))
		if err != nil {
			http.Error(w, "Failed to uncomplete task", 500)
			return
		}

		active, _ := queries.ListActiveTasks(r.Context())
		completed, _ := queries.ListCompletedTasks(r.Context())
		_ = views.TaskList(active, completed, 0).Render(r.Context(), w)
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
			http.Error(w, "Failed to delete task", 500)
			return
		}

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
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Listening on port", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
