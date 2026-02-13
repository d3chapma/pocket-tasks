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
	"github.com/go-chi/chi/middleware"
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

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := queries.ListTasks(r.Context())
		if err != nil {
			log.Println("ListTasks error:", err)
			http.Error(w, "Failed to load tasks", 500)
			return
		}

		views.Layout(
			views.TaskList(tasks),
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

		tasks, _ := queries.ListTasks(r.Context())

		views.TaskList(tasks).Render(r.Context(), w)
	})

	r.Post("/tasks/toggle/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")

		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}

		_, err = queries.ToggleTask(r.Context(), int32(id))
		if err != nil {
			http.Error(w, "Failed to toggle task", 500)
			return
		}

		tasks, _ := queries.ListTasks(r.Context())

		views.TaskList(tasks).Render(r.Context(), w)
	})

	r.Post("/tasks/delete/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := chi.URLParam(r, "id")

		id, err := strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "Invalid ID", 400)
			return
		}

		err = queries.DeleteTask(r.Context(), int32(id))
		if err != nil {
			http.Error(w, "Failed to delete task", 500)
			return
		}

		tasks, _ := queries.ListTasks(r.Context())

		views.TaskList(tasks).Render(r.Context(), w)
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Listening on port", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
