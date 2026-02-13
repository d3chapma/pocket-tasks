# Pocket Tasks

A minimal task manager built with:

- **Go**
- **sqlc** (type-safe SQL)
- **templ** (type-safe HTML templates)
- **Datastar** (HTML-over-the-wire interactivity)
- **PostgreSQL**

This app demonstrates a modern server-driven architecture with zero frontend frameworks.

---

# Requirements

- Go 1.22+
- PostgreSQL
- `sqlc`
- `templ`

Install tools if needed:

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/a-h/templ/cmd/templ@latest
```

---

# Project Structure

```
cmd/server/        → Application entrypoint
internal/db/       → sqlc generated database layer
internal/views/    → templ components
db/schema.sql      → Database schema
db/query.sql       → SQL queries for sqlc
```

---

# Setup

## 1. Create the Database

Create a local PostgreSQL database:

```bash
createdb pocket
```

Or manually:

```bash
psql postgres
CREATE DATABASE pocket;
\q
```

---

## 2. Apply the Schema

Run:

```bash
psql -d pocket -f db/schema.sql
```

Verify:

```bash
psql pocket
\dt
```

You should see:

```
tasks
```

---

## 3. Set DATABASE_URL

Set the environment variable:

```bash
export DATABASE_URL="postgres://USER:PASSWORD@localhost:5432/pocket?sslmode=disable"
```

Replace `USER` and `PASSWORD` with your local Postgres credentials.

---

# Code Generation

This project uses two generators:

## Generate SQL Code (sqlc)

```bash
sqlc generate
```

This reads:

- `db/schema.sql`
- `db/query.sql`

And generates type-safe Go code in:

```
internal/db/
```

---

## Generate Templates (templ)

```bash
templ generate
```

This compiles `.templ` files into `.go` files inside:

```
internal/views/
```

---

# Running the Server

Start the app:

```bash
go run ./cmd/server
```

Open:

```
http://localhost:8080
```

You should be able to:

- Add tasks
- Toggle tasks
- Delete tasks

All without page reloads.

---

# How It Works

## Backend

- `chi` router handles HTTP requests
- `pgxpool` manages database connections
- `sqlc` generates type-safe query methods
- Handlers call queries and render templ components

No ORM is used.

---

## Frontend

- `templ` generates type-safe HTML components
- `Datastar` intercepts events (`@post`)
- Server returns HTML fragments
- Datastar replaces DOM targets

There is:

- No JSON API
- No frontend build step
- No JavaScript framework

---

# Development Workflow

After editing:

- `db/schema.sql` or `db/query.sql` → run `sqlc generate`
- `.templ` files → run `templ generate`

Then rebuild:

```bash
go build ./...
```

---

# Why This Stack?

| Tool       | Purpose                              |
| ---------- | ------------------------------------ |
| Go         | Fast, simple backend                 |
| sqlc       | Compile-time SQL safety              |
| templ      | Type-safe HTML                       |
| Datastar   | Interactive UI without JS frameworks |
| PostgreSQL | Reliable relational database         |

This results in:

- Simple architecture
- Small deployable binary
- Fast startup
- Minimal operational complexity

---

# Future Improvements

- Database migrations
- Docker container
- Cloud Run deployment
- Cloud SQL integration
- Authentication
- Structured logging

---

# License

MIT
