# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Parro is a Go CLI that syncs school announcements, calendar events, and chat messages from the Parro parent-school communication platform into a local SQLite database. It reverse-engineers the Parro REST API v2.

## Commands

```bash
go test ./...          # Build check + run all tests (never use go build)
go test ./internal/db  # Run tests for a single package
go test -run TestName ./internal/api  # Run a single test
```

No Makefile, no linter configured. Pre-commit: run `go fix ./...`.

## Architecture

```
cmd/parro/main.go       → CLI entry point (login, init, check subcommands)
internal/api/client.go  → HTTP client with OAuth2 token refresh, guardian auth header
internal/api/login.go   → 6-step OAuth2+PKCE login flow (Wicket form navigation)
internal/api/types.go   → JSON response structs (Account, Group, Announcement, etc.)
internal/db/db.go       → SQLite store: schema, migrations, upsert, queries
doc/api.md              → Reverse-engineered API specification
```

**Data flow (`check` command):** load tokens from DB → refresh access token (rolling) → fetch groups → announcements per group → calendar events → chatrooms → messages → upsert all to SQLite → print new items since last sync.

## Key patterns

- **API client:** generic `getList[T]()` for typed JSON list responses. HTTP client is injectable for testing.
- **Auth:** content-type `application/vnd.topicus.geon+json;version=216`, guardian header `parro-authorization-role: GUARDIAN:{id}`, rolling refresh tokens (each use invalidates previous).
- **DB:** pure-Go SQLite (`modernc.org/sqlite`), no CGO. Dedup via API IDs as PRIMARY KEY + `INSERT OR IGNORE`. Idempotent migrations via duplicate column detection. `synced_at` timestamps for incremental sync.
- **Logger:** optional `*log.Logger` field on Client, nil-safe `logf()` method. `-v` logs to stderr, `-vv` adds summary.
- **Tests:** standard `testing` package with `httptest.Server` mocks for API, real SQLite for DB.
