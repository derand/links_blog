# Links blog

A small blog of links grouped by day. It is being rewritten in Go with SQLite;
the previous Flask + MongoDB version is tagged `v1.0.0-flask-mongo`.

## Layout

```
cmd/linksblog/       the service binary
internal/auth/       Google sign-in, signed session cookie
internal/links/      link rules: URL normalization (dedup key), days, search query syntax
internal/store/      SQLite storage; migrations/ are applied on open
internal/web/        pages, templates and static files (embedded into the binary)
tools/import-mongo/  one-off import from the old MongoDB (not part of the binary)
```

## Development

Requires Go 1.27+. No CGO: SQLite is `modernc.org/sqlite`.

```sh
go test ./...
go run ./cmd/linksblog migrate          # creates data/linksblog.db
LINKSBLOG_SESSION_SECRET=$(openssl rand -hex 32) go run ./cmd/linksblog serve -addr 127.0.0.1:8080
```

`linksblog serve -h` lists the settings. Sign-in needs `LINKSBLOG_BASE_URL`,
`LINKSBLOG_ADMIN_EMAILS`, `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`; the Google
OAuth client must allow the redirect URI `<base URL>/oauth2callback`.

The database path is set with `-db` or `LINKSBLOG_DB` (default `data/linksblog.db`).
`data/` is git-ignored.

## Importing the old MongoDB data

```sh
mongoexport --uri "$MONGODB_CONNECTION" --collection links --jsonArray --out data/links.json
go run ./tools/import-mongo -in data/links.json -db data/linksblog.db
```

The import needs an empty database. It converts the old `YYYYDDD` day numbers to
`YYYY-MM-DD`, keeps the Mongo `_id` in `source_ref`, skips duplicate URLs (keeping
the first one) and prints a warning for every record it skips or changes.
