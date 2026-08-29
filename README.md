# Tempo fitness tracker

A focused, local-first personal fitness tracker built with Go, Templ, HTMX, and PostgreSQL.

## Run locally

Docker Desktop is the only prerequisite.

```sh
docker compose up --build
```

Open [http://localhost:8080](http://localhost:8080). The database is stored in the named `fitness_data` volume and survives container restarts. Stop the application with `docker compose down`; add `-v` only when you intentionally want to erase all fitness data.


The application applies embedded, transactional SQL migrations on startup and seeds a small reusable exercise library. It is designed for one local user and has no login screen.

## Development

With Go 1.24 and Templ installed:

```sh
templ generate
go test ./...
go run ./cmd/fitness-tracker
```

The default development database URL is `postgres://fitness:fitness@localhost:5432/fitness?sslmode=disable`; override it with `DATABASE_URL`. Set `HTTP_ADDR` to change the listen address.

## Included workflows

- Shared weighted and bodyweight exercise library
- One active routine with ordered workout days
- Deletable workout days that preserve completed-session history
- Per-exercise sets × reps and EMOM blocks, freely mixed within one workout day
- Optional rep targets that never limit or overwrite the actual reps logged in a session
- Autosaved workout drafts and editable completed-session logs backed by stable workout snapshots
- Compact EMOM logging: apply shared reps and optional weight to non-skipped minutes, then expand to edit individual exceptions
- Routine consistency and routine-independent per-exercise progress analytics
- Exercise-specific weighted volume (kg·reps) and bodyweight rep tracking
