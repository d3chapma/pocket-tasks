package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

type contextKey string

const userKey contextKey = "user"

// --- Session cookie ---

func signSession(userID int32, secret string) string {
	msg := strconv.Itoa(int(userID))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return msg + "." + hex.EncodeToString(mac.Sum(nil))
}

func verifySession(value, secret string) (int32, bool) {
	dot := strings.LastIndex(value, ".")
	if dot < 0 {
		return 0, false
	}
	msg, sig := value[:dot], value[dot+1:]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	if !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return 0, false
	}
	id, err := strconv.Atoi(msg)
	if err != nil {
		return 0, false
	}
	return int32(id), true
}

// --- Auth middleware ---

func authMiddleware(queries db.Querier, sessionSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			userID, ok := verifySession(cookie.Value, sessionSecret)
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			user, err := queries.GetUserByID(r.Context(), userID)
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func currentUser(r *http.Request) db.User {
	return r.Context().Value(userKey).(db.User)
}

// --- Mailgun email ---

func sendMagicLink(apiKey, domain, fromEmail, toEmail, link string) error {
	endpoint := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", domain)
	body := strings.NewReader(fmt.Sprintf(
		"from=%s&to=%s&subject=Sign+in+to+Pocket+Tasks&text=%s",
		fromEmail,
		toEmail,
		"Click+the+link+below+to+sign+in+to+Pocket+Tasks.%0A%0A"+link+"%0A%0AThis+link+expires+in+15+minutes.+If+you+didn%27t+request+this%2C+you+can+ignore+this+email.",
	))
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth("api", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mailgun returned status %d", resp.StatusCode)
	}
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- Task helpers ---

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func prevDateFor(ctx context.Context, queries db.Querier, userID int32, day time.Time) string {
	_, err := queries.GetPrevCompletedDate(ctx, db.GetPrevCompletedDateParams{Column1: day, UserID: userID})
	if err == nil {
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

func loadHistoryDays(ctx context.Context, queries db.Querier, userID int32, historyOldest string) ([]views.CompletedDay, string) {
	if historyOldest == "" {
		return nil, ""
	}
	oldest, err := time.Parse("2006-01-02", historyOldest)
	if err != nil {
		return nil, ""
	}
	oldestDay := startOfDay(oldest)
	prevRow, err := queries.GetPrevCompletedDate(ctx, db.GetPrevCompletedDateParams{Column1: oldestDay, UserID: userID})
	if err != nil {
		return nil, ""
	}
	newOldest := startOfDay(prevRow)
	today := startOfDay(time.Now().UTC())
	historical, _ := queries.ListHistoricalCompletedTasks(ctx, db.ListHistoricalCompletedTasksParams{
		Column1: newOldest,
		Column2: today,
		UserID:  userID,
	})
	return groupTasksByDay(historical), prevDateFor(ctx, queries, userID, newOldest)
}

func renderTasks(w http.ResponseWriter, r *http.Request, queries db.Querier, userID int32, selectedIndex int) {
	ctx := r.Context()
	active, _ := queries.ListActiveTasks(ctx, userID)
	completed, _ := queries.ListCompletedTasks(ctx, userID)

	total := len(active) + len(completed)
	if selectedIndex >= total {
		selectedIndex = total - 1
	}
	if selectedIndex < 0 {
		selectedIndex = 0
	}

	today := startOfDay(time.Now().UTC())
	historyOldest := r.URL.Query().Get("historyOldest")
	historyDays, histPrevDate := loadHistoryDays(ctx, queries, userID, historyOldest)
	prevDate := prevDateFor(ctx, queries, userID, today)
	if historyOldest != "" {
		prevDate = histPrevDate
	}

	_ = views.TaskList(active, completed, selectedIndex, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
}

// --- DB setup ---

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
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		log.Fatal("SESSION_SECRET is not set")
	}
	mailgunAPIKey := os.Getenv("MAILGUN_API_KEY")
	mailgunDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := os.Getenv("FROM_EMAIL")
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
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

	r.Get("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/manifest.json")
	})
	r.Get("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "static/sw.js")
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// --- Auth routes (unauthenticated) ---

	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		hasError := r.URL.Query().Get("error") == "1"
		_ = views.Login(false, hasError).Render(r.Context(), w)
	})

	r.Post("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", 400)
			return
		}
		email := strings.TrimSpace(r.FormValue("email"))
		if email == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		user, err := queries.GetOrCreateUser(r.Context(), email)
		if err != nil {
			log.Println("GetOrCreateUser error:", err)
			http.Error(w, "Server error", 500)
			return
		}

		token, err := generateToken()
		if err != nil {
			http.Error(w, "Server error", 500)
			return
		}

		expiresAt := time.Now().UTC().Add(15 * time.Minute)
		if err := queries.CreateAuthToken(r.Context(), db.CreateAuthTokenParams{
			Token:     token,
			UserID:    user.ID,
			ExpiresAt: expiresAt,
		}); err != nil {
			log.Println("CreateAuthToken error:", err)
			http.Error(w, "Server error", 500)
			return
		}

		magicLink := fmt.Sprintf("%s/auth/verify?token=%s", baseURL, token)

		if mailgunAPIKey != "" && mailgunDomain != "" && fromEmail != "" {
			if err := sendMagicLink(mailgunAPIKey, mailgunDomain, fromEmail, email, magicLink); err != nil {
				log.Println("sendMagicLink error:", err)
				http.Error(w, "Failed to send email", 500)
				return
			}
		} else {
			// Dev mode: log the link instead of emailing
			log.Printf("Magic link for %s: %s\n", email, magicLink)
		}

		_ = views.Login(true, false).Render(r.Context(), w)
	})

	r.Get("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}

		authToken, err := queries.GetValidAuthToken(r.Context(), token)
		if err != nil {
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}

		if err := queries.MarkAuthTokenUsed(r.Context(), token); err != nil {
			log.Println("MarkAuthTokenUsed error:", err)
			http.Error(w, "Server error", 500)
			return
		}

		sessionValue := signSession(authToken.UserID, sessionSecret)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    sessionValue,
			Path:     "/",
			HttpOnly: true,
			Secure:   strings.HasPrefix(baseURL, "https"),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24 * 30, // 30 days
		})
		http.Redirect(w, r, "/", http.StatusFound)
	})

	r.Post("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		http.Redirect(w, r, "/login", http.StatusFound)
	})

	// --- Task routes (authenticated) ---

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware(queries, sessionSecret))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			user := currentUser(r)
			active, err := queries.ListActiveTasks(r.Context(), user.ID)
			if err != nil {
				log.Println("ListActiveTasks error:", err)
				http.Error(w, "Failed to load tasks", 500)
				return
			}
			completed, err := queries.ListCompletedTasks(r.Context(), user.ID)
			if err != nil {
				log.Println("ListCompletedTasks error:", err)
				http.Error(w, "Failed to load tasks", 500)
				return
			}

			today := startOfDay(time.Now().UTC())
			_ = views.Layout(
				views.TaskList(active, completed, 0, "Completed Today", prevDateFor(r.Context(), queries, user.ID, today), nil, ""),
				user.Email,
			).Render(r.Context(), w)
		})

		r.Post("/tasks", func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid Form", 400)
				return
			}

			user := currentUser(r)
			title := r.FormValue("title")
			selectedIndex, _ := strconv.Atoi(r.URL.Query().Get("selectedIndex"))
			historyOldest := r.URL.Query().Get("historyOldest")

			active, _ := queries.ListActiveTasks(r.Context(), user.ID)

			var insertAfterIdx int
			if selectedIndex < len(active) {
				insertAfterIdx = selectedIndex
			} else {
				insertAfterIdx = len(active) - 1
			}

			var newPosition int32
			if len(active) == 0 {
				newPosition = 1
			} else if insertAfterIdx == len(active)-1 {
				newPosition = active[insertAfterIdx].Position + 1
			} else {
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
				UserID:   user.ID,
			})
			if err != nil {
				http.Error(w, "Failed to create task", 500)
				return
			}

			active, _ = queries.ListActiveTasks(r.Context(), user.ID)
			completed, _ := queries.ListCompletedTasks(r.Context(), user.ID)

			newSelectedIndex := insertAfterIdx + 1
			if len(active) == 1 {
				newSelectedIndex = 0
			}

			ctx := r.Context()
			today := startOfDay(time.Now().UTC())
			historyDays, histPrevDate := loadHistoryDays(ctx, queries, user.ID, historyOldest)
			prevDate := prevDateFor(ctx, queries, user.ID, today)
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
			user := currentUser(r)
			historyOldest := r.URL.Query().Get("historyOldest")

			_, err = queries.CompleteTask(r.Context(), int32(id))
			if err != nil {
				http.Error(w, "Failed to complete task", 500)
				return
			}

			ctx := r.Context()
			active, _ := queries.ListActiveTasks(ctx, user.ID)
			completed, _ := queries.ListCompletedTasks(ctx, user.ID)

			newIndex := len(active)
			total := len(active) + len(completed)
			if newIndex >= total {
				newIndex = total - 1
			}

			today := startOfDay(time.Now().UTC())
			historyDays, histPrevDate := loadHistoryDays(ctx, queries, user.ID, historyOldest)
			prevDate := prevDateFor(ctx, queries, user.ID, today)
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
			user := currentUser(r)
			historyOldest := r.URL.Query().Get("historyOldest")

			maxPos, _ := queries.GetMaxPosition(r.Context(), user.ID)
			_, err = queries.UncompleteTask(r.Context(), db.UncompleteTaskParams{
				ID:       int32(id),
				Position: maxPos + 1,
			})
			if err != nil {
				http.Error(w, "Failed to uncomplete task", 500)
				return
			}

			ctx := r.Context()
			active, _ := queries.ListActiveTasks(ctx, user.ID)
			completed, _ := queries.ListCompletedTasks(ctx, user.ID)

			newIndex := len(active) - 1
			if newIndex < 0 {
				newIndex = 0
			}

			today := startOfDay(time.Now().UTC())
			historyDays, histPrevDate := loadHistoryDays(ctx, queries, user.ID, historyOldest)
			prevDate := prevDateFor(ctx, queries, user.ID, today)
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
			user := currentUser(r)
			direction := chi.URLParam(r, "direction")

			active, _ := queries.ListActiveTasks(r.Context(), user.ID)

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
					renderTasks(w, r, queries, user.ID, idx)
					return
				}
				newIdx = idx - 1
			case "down":
				if idx == len(active)-1 {
					renderTasks(w, r, queries, user.ID, idx)
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

			task := active[idx]
			active = slices.Delete(active, idx, idx+1)
			active = slices.Insert(active, newIdx, task)

			for i, t := range active {
				_ = queries.UpdateTaskPosition(r.Context(), db.UpdateTaskPositionParams{
					ID:       t.ID,
					Position: int32(i + 1),
				})
			}

			ctx := r.Context()
			historyOldest := r.URL.Query().Get("historyOldest")
			completed, _ := queries.ListCompletedTasks(ctx, user.ID)
			today := startOfDay(time.Now().UTC())
			historyDays, histPrevDate := loadHistoryDays(ctx, queries, user.ID, historyOldest)
			prevDate := prevDateFor(ctx, queries, user.ID, today)
			if historyOldest != "" {
				prevDate = histPrevDate
			}

			_ = views.TaskList(active, completed, newIdx, "Completed Today", prevDate, historyDays, historyOldest).Render(ctx, w)
		})

		r.Get("/tasks/history", func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			user := currentUser(r)

			oldestStr := r.URL.Query().Get("oldest")
			oldest, err := time.Parse("2006-01-02", oldestStr)
			if err != nil {
				http.Error(w, "Invalid date", http.StatusBadRequest)
				return
			}

			oldestDay := startOfDay(oldest)
			prevRow, err := queries.GetPrevCompletedDate(ctx, db.GetPrevCompletedDateParams{Column1: oldestDay, UserID: user.ID})
			if err != nil {
				http.Error(w, "No previous day found", http.StatusNotFound)
				return
			}

			newOldest := startOfDay(prevRow)
			today := startOfDay(time.Now().UTC())

			historical, _ := queries.ListHistoricalCompletedTasks(ctx, db.ListHistoricalCompletedTasksParams{
				Column1: newOldest,
				Column2: today,
				UserID:  user.ID,
			})
			historyDays := groupTasksByDay(historical)

			active, _ := queries.ListActiveTasks(ctx, user.ID)
			completed, _ := queries.ListCompletedTasks(ctx, user.ID)

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

			_ = views.TaskList(active, completed, selectedIndex, "Completed Today", prevDateFor(ctx, queries, user.ID, newOldest), historyDays, oldestStr).Render(ctx, w)
		})

		r.Delete("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
			idParam := chi.URLParam(r, "id")
			id, err := strconv.Atoi(idParam)
			if err != nil {
				http.Error(w, "Invalid ID", 400)
				return
			}
			user := currentUser(r)
			selectedIndex, _ := strconv.Atoi(r.URL.Query().Get("selectedIndex"))

			err = queries.DeleteTask(r.Context(), int32(id))
			if err != nil {
				fmt.Println("Failed to delete task:", err)
				http.Error(w, "Failed to delete task", 500)
				return
			}

			renderTasks(w, r, queries, user.ID, selectedIndex)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Listening on port", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
