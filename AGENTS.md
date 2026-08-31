# Repository Guidelines

## Project Structure & Module Organization

Tempo is a local-first fitness tracker using Go, Templ, HTMX, and PostgreSQL.

- `cmd/fitness-tracker/`: application entry point and startup configuration.
- `internal/domain/`: fitness models, validation, and calculations.
- `internal/store/`: PostgreSQL persistence and analytics; `migrations/` contains embedded, numbered SQL migrations applied on startup.
- `internal/web/`: HTTP handlers, presentation helpers, and `.templ` views.
- `static/`: CSS, JavaScript, vendored HTMX and its license, and JavaScript tests.

Go tests live beside implementation files as `*_test.go`. Generated `*_templ.go` files are ignored; edit `.templ` sources instead.

## Build, Test, and Development Commands

Run commands from the repository root. Native development requires Go 1.24, Templ v0.3.960, and a reachable PostgreSQL database. Node.js is needed only for JavaScript tests.

- `docker compose up --build`: build and start the application and database at `http://localhost:8080`.
- `templ generate`: generate Go views before building or testing, and after template changes.
- `go run ./cmd/fitness-tracker`: start the application against the configured database.
- `go build -o bin/fitness-tracker ./cmd/fitness-tracker`: compile the application.
- `go test ./...`: run Go tests.
- `node --test static/app.test.cjs`: run JavaScript interaction tests.

## Coding Style & Naming Conventions

Format changed Go files with `gofmt` and templates with `templ fmt`. Use tabs in Go/Templ and two-space indentation in JavaScript/CSS. Follow Go exported `PascalCase` and unexported `camelCase` names; use descriptive lowercase filenames. Name migrations sequentially, such as `007_description.sql`. No separate lint configuration is checked in. Preserve server-rendered forms and functionality without JavaScript.

## Testing Guidelines

Use Go's `testing` package and `net/http/httptest`; name tests `TestBehavior`. JavaScript tests use `node:test` and `node:assert/strict`. Add regression coverage for changed behavior; no numeric coverage threshold is configured.

Set `TEST_DATABASE_URL` to a disposable PostgreSQL database to enable integration tests. Tests create and remove temporary schemas, so the role needs schema-creation permission. Never use production data.

## Commit & Pull Request Guidelines

Recent commits use prefixes such as `feat:`, `fix:`, and `refactor:` with short imperative descriptions. Follow that pattern. PRs should describe the problem, behavior changes, and validation performed; link relevant issues and include screenshots for UI changes. Call out schema or configuration changes.

## Security & Configuration

Configure `DATABASE_URL` and `HTTP_ADDR` through the environment; keep secrets out of Git. The app has no authentication and is intended for local use. `docker compose down -v` permanently deletes the fitness database volume; omit `-v` to preserve data.
