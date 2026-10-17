# Links blog

A small blog of links grouped by day. It is being rewritten in Go with SQLite;
the previous Flask + MongoDB version is tagged `v1.0.0-flask-mongo`.

## Layout

```
cmd/linksblog/       the service binary
internal/auth/       Google sign-in, signed session cookie
internal/bot/        chat message parsing shared by bots; discord/ is the Discord bot
internal/links/      link rules: URL normalization (dedup key), days, search query syntax
internal/service/    LinkService: adding links (with dedup) and bot channels; used by bots
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

## Running with Docker

CI publishes the image `ghcr.io/derand/links_blog` for `linux/amd64` and
`linux/arm64`; `:latest` follows master.

```sh
mkdir linksblog && cd linksblog
curl -fsSLO https://raw.githubusercontent.com/derand/links_blog/master/compose.yaml
curl -fsSL https://raw.githubusercontent.com/derand/links_blog/master/.env.example -o .env
# fill in .env
mkdir data && sudo chown 65532:65532 data
docker compose up -d
```

The container runs as an unprivileged user (UID 65532), so `data/` must be
writable by it. The database is created and migrated on start; to use an existing
one, put it at `data/linksblog.db` first.

The site listens on `127.0.0.1:8080` and expects a reverse proxy that terminates
TLS in front of it; set `LINKSBLOG_BASE_URL` to the public `https://` URL. For
another setup, such as a proxy in its own container, put the changes into
`compose.override.yaml` next to `compose.yaml`.

Update with `docker compose pull && docker compose up -d`. To run a particular
commit, start the CI workflow by hand ("Run workflow", or
`gh workflow run ci.yaml -f ref=<commit>`): it publishes `:sha-<commit>`, which is
then set as `LINKSBLOG_VERSION` in `.env`. A local build: `docker build -t linksblog .`.

## Discord bot

The bot runs inside `linksblog serve` when `DISCORD_TOKEN` is set. It reads
messages of the users in `DISCORD_ALLOWED_USERS` (comma-separated user IDs) in
channels turned on for it, adds every URL as a link and reacts with ✅ (added),
🔁 (already in the blog) or ❌ (error). The rest of the text becomes the
description, `#hashtags` become tags (`_` reads as a space), and Discord's link
previews fill in the title. Edits and deletions of messages are not followed.

Setup in the [Developer Portal](https://discord.com/developers/applications):

1. New Application. On Installation: Guild Install only, Install Link: None.
2. On Bot: Public Bot off, Message Content Intent on; Reset Token gives `DISCORD_TOKEN`.
3. OAuth2 → URL Generator: scopes `bot` and `applications.commands`, permissions
   View Channels, Read Message History and Add Reactions. Open the URL to add the
   bot to a server.
4. Your user ID: Discord settings → Advanced → Developer Mode, then right-click
   your name → Copy User ID.

Commands, for the allowed users (Discord shows them to members with Manage Server):

```
/links channel set [tag:<tag>] [hidden:<bool>]   turn this channel on; tag:- removes the tag
/links channel show                              show the channel's settings
/links channel off                               turn it off
/links import                                    add links from the channel's whole history
```

Messages sent while the bot was offline are picked up when it reconnects.

## Importing the old MongoDB data

```sh
mongoexport --uri "$MONGODB_CONNECTION" --collection links --jsonArray --out data/links.json
go run ./tools/import-mongo -in data/links.json -db data/linksblog.db -tz Europe/Kyiv
```

The import needs an empty database. It converts the old `YYYYDDD` day numbers to
`YYYY-MM-DD`, keeps the Mongo `_id` in `source_ref`, skips duplicate URLs (keeping
the first one) and prints a warning for every record it skips or changes.
