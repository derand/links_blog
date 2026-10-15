# Links blog

A small blog of links grouped by day. It is being rewritten in Go with SQLite;
the previous Flask + MongoDB version is tagged `v1.0.0-flask-mongo`.

## Layout

```
cmd/linksblog/       the service binary
internal/links/      link rules: URL normalization (dedup key), days
internal/store/      SQLite storage; migrations/ are applied on open
tools/import-mongo/  one-off import from the old MongoDB (not part of the binary)
```

## Development

Requires Go 1.27+. No CGO: SQLite is `modernc.org/sqlite`.

```sh
go test ./...
go run ./cmd/linksblog migrate          # creates data/linksblog.db
```

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
