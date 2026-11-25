# Tide

A note-taking app, Obsidian-like: one Go module at the root (`github.com/cyril-fl/tide`, CLI and HTTP API) and a Nuxt web app (`web/`) on PostgreSQL, used through Tailscale from a phone and a tablet.

- `cmd/`: the two binaries (`cmd/tide/`, `cmd/tide-server/`) and the cobra wiring of the CLI. `config/`: `config.yaml`, the commands, their help and flags.
- `core/`: what runs on the server. `core/api/` (routes, see `core/api/CLAUDE.md`), `core/server/` (`serve`, the admin's commands, the MCP over HTTP and its OAuth), `core/services/process/` (the business logic, one plum Process each), `core/domain/` (the models), `core/services/` (the rest).
- `cli/`: the commands of `tide` and its MCP (`cli/cmd-mcp/`), clients of the API. See `cli/CLAUDE.md`.
- `web/`: Nuxt 4 + Nuxt UI 4, client only (`ssr: false`). See `web/CLAUDE.md`.
- `docs/swagger.yaml`: the OpenAPI, generated. `MCP.md`: the MCP for agents (in French).
- `../deprecated/` is not the previous code: ignore it. `../notes.wiki/` is the GitHub wiki (separate repo).

Two binaries. `tide-server` (`cmd/tide-server/`, `core/server/`) is the only one on the base and Silo: the API (`tide-server serve`, `core/api/`) runs the processes of `core/services/process/` on the models of `core/domain/`, and it holds the admin's commands (`user add`, `user reset-password`, `rotate-secret`). `tide` (`cmd/tide/`, `go install github.com/cyril-fl/tide/cmd/tide@latest`), the CLI (cobra, `cmd/` + `cli/`) and the MCP server for agents (`tide mcp`, `cli/cmd-mcp/`), is a client of the API only.

## Run

- `make up` / `make down`: Postgres, Silo and the API in Docker (Colima, `compose.yaml`, project `tide`), the web app on :3000 (log in `.build/web.log`).
- `make e2e`: the browser tests against that stack. Run it whole before committing a change to `web/` (with or without the checker): CI runs every spec, and a component deleted by mistake broke six of them.
- `make test` (`ARGS=./core/...` to narrow): each run gets a base `<POSTGRES_DB>_test_<pid>` and a Silo bucket of its own, removed after it, never the dev ones: agents can run it at once. Needs the stack up. A plain `go test` uses `<POSTGRES_DB>_test`, one package at a time across runs (a lock in `postgrey`). When the host port 5432 is not the stack's (a tunnel to another base), run them in the `note-app` container, the repo in `/app`: `docker exec -w /app -e POSTGRES_HOST=note-postgres -e POSTGRES_DB=notes_test -e CONFIG_FILE=/app/config/config.yaml note-app go test -p 1 ./...` (the `_test` base made once by `make test` or `createdb`).
- `make api`: rebuilds and restarts the API container (`Dockerfile`, `--target server`: no web app). The app talks to the old binary until then.
- `make openapi` after an API change (swag comments), then `bun run types` in `web/`.
- `make backup` before touching data or the schema, before deleting or migrating data.
- The CLI by hand: `set -a && . ./.env && set +a && CONFIG_FILE=$PWD/config/config.yaml go run ./cmd/tide <args>` (`./cmd/tide-server` for the server's). It acts for the account of the local CLI session: do not run `tide logout`, it deletes that session.
- CI (`.github/workflows/ci.yml`, on push and PR): gofmt, vet and the Go tests on a Postgres service; lint, Prettier, unit tests, typecheck and build of the app; swagger.yaml and api.d.ts up to date, then the e2e on the compose stack; the server image built (not pushed) and started from `compose.prod.yaml`.

## Go rules

- Processes follow the `plum-process` skill: guarded steps, `HasErr()` then `GetErr()`, `bun.IDB` so tests pass a transaction.
- Every query filters by account: `domain.UserID(ctx)`. A `bun.Tx` hands its `RunInTx` functions the ctx it began with, so begin transactions with a ctx that has the account; tests use `ctx, tx = testaccount.New(t, ctx, tx)`.
- **No migration tool.** `createschema` creates missing tables, indexes and the columns a model gained (at every start, the server too). A rename, a type change, a changed default or data to rewrite goes into the one-time queries of `core/services/process/createschema/migrations.go` (a `migrationVx_y_z` appended to `migrations`, run once per base at the next start, never edited once shipped; a stopgap until v2). A NOT NULL column without a default on a table with rows: fill it in the same migration. `RETURNING` lists the model's columns (`?Columns`), never `*`: an old column left in a base must not break a scan.
- Sentinel errors are exported and wrapped with `%w`; the API maps them to statuses in `core/api/services/respond/respond.go`.
- Periodic cleanups go into the `cleanup` of `tide-server serve` (`core/server/cmd-serve/cmd.go`, with `purgeitems` and `purgefiles`), run at startup then every hour.
- Images: a row in `files`, the bytes in Silo (S3, `compose.yaml`) through `services/silo.Store`, cited by notes as `/api/files/<id>`. Processes take the store as a parameter, tests pass `silo.NewMemory()`. `purgefiles` deletes what no note cites.
- Domain structs: `bun.BaseModel` gets `json:"-"`, fields snake_case `json` tags (they are the API's JSON and the app's types).

## Working with the user

- Talk to the user in French. Code, comments, commit messages and tag messages in English (older tags in French are no model); READMEs and wiki in English.
- One commit per change (`feat:`/`fix:`/`refactor:`/`test:`/`docs:`/`chore:`/`style:`, checked by the lefthook `commit-msg` hook: `make setup`), an annotated tag per version, the user pushes. Details and issue handling: the `tide-release` skill.
- The wiki is updated only when the user asks (`tide-wiki` skill, which catches up every tag since the last one it knows), never as part of a fix, a tag or an issue; it is pushed in `../notes.wiki` (the only push the agent does).
- Work left for later goes into GitHub issues (`gh issue list -R cyril-fl/tide`), and known limits into `TODO:` comments in the code (`TODO: <the limit>, <when to fix it>`), not `shortcut:`.
- Several agents share this checkout: files left uncommitted by another one are not yours (stage only your own hunks, never a shared file whole), and the branch may have been switched under you (`git branch --show-current` before committing; work goes on `develop`, `main` follows it at releases).
- `AGENTS.md` files are links to these `CLAUDE.md`: edit the `CLAUDE.md`.

## Skills

`tide-cli-command`, `tide-account-setting`, `tide-app-screenshot`, `tide-release`, `tide-wiki`, `tide-issue-done`, `tide-issue-hunt` (audit, then the missing issues), `plum-process`, `go-documentation`, `go-tests`, `primitive-first-review`.

Subagents (`.claude/agents/`): `tide-auditor`, a read-only audit of the code used by `tide-issue-hunt`; `tide-release-checker` (Sonnet), the checks, tag, issues and handoff of `tide-release` once the commits are made.
