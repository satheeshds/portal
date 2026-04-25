# Copilot Instructions

## Project Overview

This is a Go-based accounting REST API for managing accounts, contacts, bills, invoices, transactions, payouts, and recurring payments. It uses DuckDB as the embedded database and the Chi router for HTTP routing.

## Architecture

- **`main.go`** — Application entry point, router setup, and route registration.
- **`handlers/`** — HTTP handler functions, one file per resource. All share a global `DB *sql.DB` set by `main.go`.
- **`models/`** — Data model structs, input validation, and the `Money` type.
- **`db/`** — Database connection (`db.go`) and schema migrations (`migrations.go`).
- **`static/`** — Embedded static files served at `/`.
- **`docs/`** — Auto-generated Swagger documentation (do not edit manually).

## Build, Test, and Run

```bash
# Build
go build ./...

# Run tests
go test ./...

# Generate Swagger docs (requires swag installed)
swag init -g main.go --dir .

# Run locally
go run .
```

Environment variables:
- `DB_PATH` — Path to the DuckDB database file (default: `./data/accounting.db`)
- `PORT` — HTTP port (default: `8080`)
- `LOG_LEVEL` — Log verbosity: `debug` or `info` (default: `info`)
- `AUTH_USER` / `AUTH_PASS` — HTTP Basic Auth credentials (if unset, auth is disabled)

## Coding Conventions

### Money / Currency

- All monetary amounts are stored and transmitted as **paise** (integer, the smallest currency unit, i.e., 1/100 of a rupee).
- Use the `models.Money` type (`int64` alias) for all monetary fields.
- JSON input accepts a number or string in rupees (e.g., `12.34`) and converts to paise (×100, rounded).
- JSON output is always an integer in paise (e.g., `1234`).

### API Response Envelope

All API responses use a standard JSON envelope:

```json
{ "data": <payload>, "error": "<message>" }
```

- Use `writeJSON(w, statusCode, data)` for success responses.
- Use `writeError(w, statusCode, message)` for error responses.
- Never write raw JSON directly in handlers.

### Handler Pattern

Each resource follows this pattern:

1. A `scan<Resource>` helper function that scans a database row into a model struct.
2. A `get<Resource>ByID` helper that fetches a single row by ID.
3. Handler functions: `List<Resource>`, `Get<Resource>`, `Create<Resource>`, `Update<Resource>`, `Delete<Resource>`.
4. Each handler validates input via the model's `Validate()` method, returns `400` on bad input, `404` when not found, and `500` on database errors.

### Database

- Uses **DuckDB** via `github.com/duckdb/duckdb-go/v2`.
- `DATE` column values scan into `*string` (not `time.Time`).
- All tables use integer sequences for auto-increment primary keys (DuckDB requirement).
- Migrations are idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`).
- Use parameterized queries (`?` placeholders) — never interpolate user input into SQL.

### Swagger Documentation

- All public handler functions must have Swagger annotations (`// @Summary`, `// @Tags`, `// @Param`, `// @Success`, `// @Failure`, `// @Router`, `// @Security`).
- After changing handler signatures or adding new routes, regenerate docs with `swag init`.

### Route Registration

- All API routes are registered under `/api/v1` with `BasicAuth` middleware in `main.go`.
- New resources need both handler registration in `main.go` and the corresponding handler file in `handlers/`.

## Payment Matching

Transaction–document matching uses:
- `GET /transactions/{id}/match-suggestions` — Suggest bills/invoices/payouts that could match a transaction.
- `POST /transactions/{id}/auto-match` — Automatically link a transaction to matching documents (confidence ≥ 0.7).
- Reverse suggestions (doc → transactions): `GET /bills/{id}/match-suggestions`, `GET /invoices/{id}/match-suggestions`, `GET /payouts/{id}/match-suggestions`, `GET /recurring-payments/{id}/match-suggestions` (informational only, `linkable: false`).
