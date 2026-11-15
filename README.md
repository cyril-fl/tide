# Notes

**Notes** is a markdown note-taking app, a bit like Obsidian: notes filed in folders, connected by their `[[links]]` and `#tags`.

A Go server on PostgreSQL serves the API and the web app. A CLI and an MCP server let scripts and agents use that API. The web app is meant to be opened from a phone or a tablet, for example through Tailscale.

- **Repository:** https://github.com/cyril-fl/notes

## Documentation

The [wiki](https://github.com/cyril-fl/notes/wiki) details the web app screen by screen, titles and tags, themes, data and the version history.

Each part also has its own README:

- [`notes-go/`](./notes-go/README.md): the `notes-server` server, the `notes` CLI, the MCP, the API, the configuration and the backup.
- [`notes-app/`](./notes-app/README.md): the web app, its shortcuts and its settings.

## Features

- **Notes and folders**: a rich markdown editor (with a source mode), folders in a tree, pinning, favorites and a trash.
- **Links**: `[[links]]`, backlinks, including a note in another (`![[Note]]`), a graph of the notes and the list of broken links.
- **Tags**: `#tags` written in the text, with a page per tag.
- **Search**: a ⌘K palette that searches titles and text, smart folders, and replacing a text in one note or in all of them.
- **History**: the versions of each note, which you can compare, restore or copy into a new note, then merge back into the original.
- **Images and files**: stored in Silo (S3-compatible).
- **Accounts**: several accounts per server, by invitation, API tokens, and notes encrypted in the database.
- **Customization**: themes, light or dark mode, snippets, an adjustable sidebar and three plans (Standard, Advanced, Ultra). The settings follow the account from one device to another.
- **Import and export**: a zip of markdown files.

## Installation and usage

Notes is developed with the latest version of Go. Locally, the stack runs in Docker thanks to [`notes-go/compose.yaml`](./notes-go/compose.yaml).

```bash
git clone https://github.com/cyril-fl/notes.git
cd notes
cp notes-go/.env.example notes-go/.env   # then fill in the passwords
make -C notes-go setup                   # installs the git hooks (lefthook)
make up                                  # Postgres, Silo and the API in Docker, the web app on :3000
echo '<password>' | docker exec -i note-api /notes-server user add <name> --password-stdin
```

The web app answers on http://localhost:3000 (open `localhost`, not `127.0.0.1`), and its logs go to `.build/notes-app.log`. To stop it, `make down` (Colima stays running).

For the CLI, on the Mac:

```bash
go install github.com/cyril-fl/notes/notes-go/cmd/notes@latest
notes login https://notes.example.ts.net
```

### Requirements

- [Go](https://go.dev/) (version given in `notes-go/go.mod`)
- [Bun](https://bun.sh/), for the web app
- [Docker](https://www.docker.com/) et [Docker Compose](https://docs.docker.com/compose/)
- [Colima](https://github.com/abiosoft/colima), used by `make up` on macOS
- [Plum](https://github.com/cyril-fl/plum), a utility library (CLI, configuration, processes)

### Docker environment

[`notes-go/compose.yaml`](./notes-go/compose.yaml) defines the development services:

- `postgres` (`note-postgres`): PostgreSQL 18, on `127.0.0.1:5432`.
- `silo` (`note-silo`): image storage, on `127.0.0.1:9000` (console on `9001`).
- `api` (`note-api`): `notes-server serve`, on http://localhost:8080 (profile `api`, started by `make up`).
- `libredb` (`note-libredb`): LibreDB Studio, a web interface to explore the database, on http://localhost:3001.
- `app` (`note-app`): a Go container, to run Go against the stack.

The environment variables are described in the *Configuration* section of the [`notes-go` README](./notes-go/README.md#configuration).

## Server

Each `v*` tag publishes the `ghcr.io/cyril-fl/notes` image (amd64 and arm64), which serves the API and the web app on the same origin. On the server, you need [`compose.prod.yaml`](./compose.prod.yaml) and, next to it, a `.env` copied from [`.env.prod.example`](./.env.prod.example):

```sh
docker compose -f compose.prod.yaml up -d                                    # Postgres, Silo, Notes and LibreDB Studio
echo '<password>' | docker exec -i note-api /notes-server user add <name> --password-stdin
docker compose -f compose.prod.yaml pull && docker compose -f compose.prod.yaml up -d   # update
```

The image only contains `notes-server`, the only binary that talks to Postgres and Silo. It runs the API and the web app, and carries the admin's commands (`user add`, `user reset-password`, `rotate-secret`), which you run in the container with `docker exec`. The `notes` CLI is a client of the API.

Notes listens over HTTP on `NOTES_LISTEN` (`127.0.0.1:8080` by default). The session cookie requires HTTPS: the proxy in front provides it. `WEB_ORIGIN` is the exact address the browser types. LibreDB Studio listens on `LIBREDB_LISTEN` (`127.0.0.1:3001`): connect to `note-postgres:5432` with the `POSTGRES_USER` user.

At startup, Notes adds the tables and columns of new versions to the database. A rename or a type change is still done by hand; the commit message says so.

> ⚠️ `NOTES_SECRET` encrypts the notes. Keep a copy in a password manager, never in a backup: without this key, the notes are unreadable.

### Accounts

The first account in the database is admin. Only it invites, lists and deletes the others, from the app's settings (*Users* section). This first account is created in three ways:

- on an empty database, the app leads straight to the account creation page;
- if `NOTES_HOST_NAME` and `NOTES_HOST_PASSWORD` are set in the `.env`, Notes creates it at startup; those two lines should then be emptied;
- the `notes-server user add` command above works in every case.

After that, `NOTES_ADD_USER_MODE` decides who can create an account:

- `invitation` (default): the admin creates an invitation link in the settings and sends it. The link works once and expires after 7 days. The invitee chooses their own name and password. Without an invitation, the account creation page is closed.
- `public`: anyone who can reach the app can create an account. Reserve this mode for a trusted network (Tailscale), since nothing limits the number of creations.

Deleting an account permanently erases its notes, folders, tags and images. An admin can neither delete themselves nor delete the last admin.

## Makefile

At the root:

| Target      | Description                                                                   |
|-------------|-------------------------------------------------------------------------------|
| `make up`   | Starts Colima, Postgres, Silo and the API in Docker, then the web app on :3000 |
| `make down` | Stops the web app and the containers (Colima stays running)                   |
| `make e2e`  | Runs the browser tests (Playwright) against the stack                         |
| `make perf` | Measures the API and the pages on an account of 50,000 generated notes       |

In `notes-go/`:

| Target         | Description                                                                                    |
|----------------|------------------------------------------------------------------------------------------------|
| `make setup`   | Installs the git hooks (lefthook)                                                              |
| `make test`    | Runs the Go tests in the `<POSTGRES_DB>_test` database (`ARGS=./app/...` to target a folder)   |
| `make api`     | Rebuilds and restarts the API container                                                        |
| `make openapi` | Regenerates `docs/swagger.yaml` (then `bun run types` in `notes-app/`)                         |
| `make backup`  | Backs up the database and the images into `backups/`                                           |
| `make pgsql`   | Opens psql, or runs a query with `QUERY="..."`                                                 |
| `make connect` | Connects the CLI to an API (`TO=local` or `TO=fedora`)                                         |

## Quality

- The lefthook hooks (`make setup` in `notes-go/`) run gofmt, Prettier and actionlint (for the workflows) before each commit. They also check the message prefix (`feat:`, `fix:`…).
- The GitHub Actions CI ([`.github/workflows/ci.yml`](./.github/workflows/ci.yml)) runs on every push and every PR:
  - format, vet and the Go tests;
  - lint, format, tests, typecheck and build of the app;
  - the check that `swagger.yaml` and `api.d.ts` are up to date;
  - the e2e tests on the Docker stack;
  - the build of the server image, without publishing it.

## Acknowledgments

### Libraries

- [Cobra](https://github.com/spf13/cobra): the CLI
- [Bun](https://bun.uptrace.dev/): the ORM, on PostgreSQL
- [MinIO Go Client](https://github.com/minio/minio-go): S3 storage
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk): the MCP server
- [Nuxt](https://nuxt.com/) and [Nuxt UI](https://ui.nuxt.com/): the web app
- [Tiptap](https://tiptap.dev/): the editor
- [KaTeX](https://katex.org/) and [Mermaid](https://mermaid.js.org/): formulas and diagrams

### Services

- Silo: image storage, S3-compatible
- LibreDB Studio: a web interface for the database

## License

[MIT](./notes-go/LICENSE)
