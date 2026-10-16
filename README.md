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

## Deployment

The site runs in Docker behind a shared [Caddy](https://caddyserver.com/) that
terminates TLS for all sites of the server. Containers talk over the external
network `web`; only Caddy publishes ports 80 and 443.

```sh
docker network create web

# Caddy, once per server: copy deploy/caddy/ to /opt/caddy/, add a site block
# per site to the Caddyfile, then
cd /opt/caddy && docker compose up -d
docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile   # after edits

# The site: compose.yaml and .env (see deploy/linksblog.env.example) in /opt/linksblog/,
# the database in /opt/linksblog/data/ owned by the image's user 65532.
sudo chown -R 65532:65532 /opt/linksblog/data
cd /opt/linksblog && docker compose up -d
```

The image is built for `linux/arm64` and `linux/amd64` from the same Dockerfile
(`docker buildx build --platform linux/arm64,linux/amd64 .`). Ports 80 and 443
(and 443/udp for HTTP/3) must be open both in the host firewall and in the cloud
provider's network rules, and DNS must point at the server before Caddy starts
so it can obtain the certificate.

## Importing the old MongoDB data

```sh
mongoexport --uri "$MONGODB_CONNECTION" --collection links --jsonArray --out data/links.json
go run ./tools/import-mongo -in data/links.json -db data/linksblog.db
```

The import needs an empty database. It converts the old `YYYYDDD` day numbers to
`YYYY-MM-DD`, keeps the Mongo `_id` in `source_ref`, skips duplicate URLs (keeping
the first one) and prints a warning for every record it skips or changes.
