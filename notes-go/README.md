# notes-go

Note-taking in Go, with folders, tags and a trash: the `notes-server` server (the API, on PostgreSQL and Silo) and the `notes` CLI, its client. Notes are in markdown.

## Requirements

- Go (version given in `go.mod`)
- Docker, with Colima for `make up` and `make down`

## Installation

Two binaries come out of this module:

- `notes`, the CLI and the MCP server: a client of the API, with no access to Postgres or Silo, to install on the Mac;
- `notes-server`, the only one that talks to the database: the API and the web app (`serve`) and the admin's commands. It runs in the Docker image (see *Server* in the root README).

```sh
go install github.com/cyril-fl/notes/notes-go/cmd/notes@latest   # the latest version of main
notes login https://notes.example.ts.net
```

In the repo, to develop:

```sh
cp .env.example .env     # then fill in the passwords
make up                  # starts Colima, PostgreSQL, Silo and LibreDB Studio
make api                 # notes-server in Docker, on http://localhost:8080
go build -o "$(go env GOPATH)/bin/notes" ./cmd/notes
make connect TO=local    # notes login on the API of make api
```

`notes-server` creates the schema before each of its commands, and `serve` empties the trash at startup then every hour. The CLI never touches the database: a CLI of a different version than the server cannot create its schema there.

## Usage

### Accounts

Each note, folder and tag belongs to an account, and an account never sees those of the others. The first account in the database is admin: only it invites, lists and deletes the others from the app (see *Server* in the root README). `notes-server`, on the server, creates an account whatever the server's mode:

```sh
notes-server user add cyril     # asks for the password twice (8 characters at least)
notes-server user reset-password cyril  # forgotten password: new password, all devices logged out
notes-server rotate-secret      # re-encrypts the accounts' keys with a new NOTES_SECRET (see Encryption)
notes login https://notes.example.ts.net  # name and password on this server, then every command acts for this account
notes logout                    # also ends the session on the server
notes whoami                    # the account and the server the CLI acts for
make connect TO=fedora          # in the repo: login on NOTES_FEDORA_URL from the .env, or TO=local on the API of make api
notes account purge             # empties the logged-in account (notes, folders, tags, images), after typing its name
```

- **On the server:** in the container, `docker exec -it note-api /notes-server user add cyril` (`-i` alone with `--password-stdin`).
- **Scripts:** `--password-stdin` reads the password from standard input, for `user add` and `user reset-password`.
- **Admin:** the flag is set by the first account created in an empty database, however it is created. To move it or add one, you have to touch the database: `UPDATE users SET admin = true WHERE name = '<name>'`.
- **CLI session:** the server URL and the token are kept in `~/.config/notes/session.json`, readable by the user only. The session lasts 30 days, extended at each use. `NOTES_URL` and `NOTES_SESSION`, if set, take precedence over this file: `notes login <url> --print-token` creates a separate session and prints only its token, for an agent that cannot log in where it runs (see `MCP.md`). `notes logout` with the variable only ends that session. The token opens the whole account, it goes in no versioned file.

### API tokens

```sh
notes add token "backup script"               # prints the token once, on stdout
notes add token "Claude phone" --expires 30d   # 7d, 30d, 90d (default), 1y or never
notes show token                              # name, uuid, creation, last use, expiry
notes remove token <uuid>                     # revokes: the token stops working at once
```

An API token (`notes_…`) is sent as `Authorization: Bearer <token>`. It does everything the account does on notes, folders, tags and files, but does not manage the account: its tokens, its password, its settings, the purge, the logout and the admin routes answer 403. The database only keeps its fingerprint (SHA-256). Once expired, it is refused (401) then purged by the server's hourly cleanup. They can also be managed in Settings > Account of the app.

The `note` and `folder` commands, as well as `move`, `finder` and `graph`, accept a UUID or a path:

```sh
notes show note 3f2a...               # by UUID
notes show note /Projets/Go/Idées     # by path
```

A path must match exactly, capitals included. If several items have the same path, the error lists their UUIDs.

### Notes and folders

```sh
notes add folder --title Projets
notes add note --content "# Idées" --tag go,sql --parent /Projets
cat idees.md | notes add note --content - --parent /Projets

notes show note /Projets/Idées
notes show folder /Projets

notes edit note /Projets/Idées --tag go --untag sql
notes edit note /Projets/Idées --pin       # --unpin to remove
notes edit note /Projets/Idées --lock      # read-only, --unlock to remove
cat idees.md | notes edit note /Projets/Idées --content -
notes edit folder /Projets --title Travail
notes edit folder /Projets --pin            # as a favorite, --unpin to remove

notes move /Projets/Idées --parent /Archives
```

If a value is missing, a form opens. Without a terminal (cron, CI), pass everything through flags.

Two folders in the same folder cannot have the same name, capitals included: creating, renaming, moving or restoring a folder next to a namesake is refused (409 in the API). A database created before this rule has its duplicates renamed at startup, `Name (2)`, `Name (3)`… Notes, on the other hand, can share a title.

A note has no separate title: it is its first markdown heading (`#`, `##`…, whatever the level), otherwise the first 50 characters of its first line of text, without formatting, code or lines made only of `#tags`. A note without text stays untitled. The title follows the content at each change, and the path of a note (`/Projets/Idées`) with it.

### Browse and search

```sh
notes finder [folder]     # explore, open, edit, create, delete
notes graph [note]        # navigate between notes that share tags
notes search recette      # search titles and content
notes search recette --limit 0   # every note found, 50 without --limit
notes recent              # the last 20 modified notes
```

### Tags

```sh
notes add tag go sql
notes show tag go
notes show tag --list
notes edit tag go golang
notes remove tag sql
```

The tags of a note are the `#words` of its text, code and links excluded: `#idée` in the content creates the tag and links it to the note, and erasing it removes it. `--tag` writes `#tag` at the end of the text and `--untag` erases it. Renaming or deleting a tag fixes the text of all its notes, and a note that ends up empty goes to the trash. A tag with no note left disappears. A tag contains letters, digits, `_` or `-`, with at least one letter.

### Links between notes

Write `[[Title]]` in a note to link it to the note with that title, capitals or not, outside code. On save, a `[[Title]]` that designates a single note becomes `[Title](note:<uuid>)`: the link now follows that note by its id, even if another takes its title, and the text between brackets updates when it changes title. If no note, or several, have that title, `[[Title]]` waits: it will be linked as soon as a single note bears that title. To break a tie, write its path, `[[/Projets/Idées]]`.

`[[Title#Section]]` targets a heading of the note: it becomes `[Title#Section](note:<uuid>#Section)`, keeps the section when the note changes title, and becomes `[[Title#Section]]` again on export. In the app, typing `#` after a note's name offers its headings, and a click opens the note at that heading. `[[#Section]]` targets a heading of the note itself: it stays as is, `[[` then `#` offers the note's headings, and a click scrolls to it.

`![[Title]]`, like in Obsidian, includes the note: it becomes `![Title](note:<uuid>)` like a link, and becomes `![[Title]]` again on export. In the app, the included note is displayed read-only under a link that opens it; its own inclusions stay links, so that a note that includes itself, or includes a note that includes it, stops there.

`GET /api/notes/{id}/neighbors` gives its links (`links`) and its backlinks (`backlinks`), and the graph also connects linked notes. In the app, typing `[[` offers the notes, and a click on a link opens it. `GET /api/links/broken` lists the links of live notes that lead nowhere: `unresolved` (no other note with that title or path, or a `#Section` link to a heading the note does not have), `ambiguous` (several), `missing` (link to a note put in the trash or purged) or `file` (image or attachment `/api/files/<id>` that the account does not have: imported from another account, purged).

### Trash

```sh
notes remove note /Projets/Idées
notes remove folder /Projets      # with all its content
notes trash
notes restore <uuid>
```

`restore` only accepts UUIDs, displayed by `notes trash`. Items are deleted for good when the CLI starts, once the account's retention period has passed (30 days by default, adjustable in Settings > Trash of the app, "Never erased" to not purge).

### Export and import

```sh
notes export ~/notes-md              # all the notes
notes export ~/projets /Projets      # a folder
notes import ~/notes-md --parent /Archive
notes import ~/Obsidian/vault        # at the root
```

`export` writes into a new or empty folder, never over files: a folder becomes a directory, a note a `<title>.md` file with its text as is (title and `#tags` included). `[Title](note:<uuid>)` links become `[[Title]]` again, since ids mean nothing outside this database. Two notes with the same title in the same folder give `Title.md` and `Title (2).md`.

`import` does the opposite, all or nothing: a folder name already taken at the same place stops the whole import, leaving nothing behind. Hidden files (`.obsidian`, `.git`…) and those that are not `.md` are ignored. A text without a markdown title gets its file name at the top, `# Name`, like in Obsidian. `[[links]]` are linked at import, as when typing.

Images follow the notes. On export, those a note cites are copied into `_files/` at the root of the export, and the note cites them by a relative path (`../_files/<uuid>.png`). On import, an image cited by a relative path (`![](photo.png)`, `![](<photo de mer.png>)`) becomes an image of the note, wherever it is in the folder. `_files/` does not become a folder. An image that is missing or not PNG, JPEG, GIF or WebP keeps its link as is. A link to a file of the folder (`[rapport](rapport.docx)`) or a `![[son.mp3]]` that is not an image makes it an attachment, cited by a link; a link to a note or to a refused type stays as is. On export, a zip keeps the extension of its name (`.docx`, `.xlsx`…). Obsidian's `![[image.png]]` and `![[image.png|300]]` too: the image is looked up by name in the whole folder (the one in the note's folder first), their width becomes the closest editor size (`"small"` up to 300 px, `"medium"` up to 600, nothing beyond).

### Images

Pasting or dragging an image into the app's editor sends it to the API, which stores it in [Silo](https://github.com/pgsty/silo) (an S3 storage, a MinIO fork, started by `make up`) and describes it in the `files` table. The note cites it in markdown: `![name](/api/files/<uuid>)`. Another file (PDF, zip, hence also Office documents, MP3, WAV, Ogg, MP4, WebM; 20 MB at most) becomes an attachment, cited by a link `[name](/api/files/<uuid>)`: the API sends it as a download under its name, and it follows the same purge and export rules as an image. The avatar stays an image. `GET /api/files` lists the account's files (name, type, size), the heaviest first, without those of a purged note or erased data.

- PNG, JPEG, GIF and WebP only (no SVG, which can contain scripts), 20 MB at most. The type is read from the file, not from what the browser sends.
- An image follows its note into the trash: when the note is erased for good (period elapsed, trash emptied, or its folder purged), its images go with it at the next hourly purge, unless another note still cites them.
- An image removed from a note's text waits for the account's trash period, counted from when it is no longer cited (never if the trash keeps everything): until then, undoing or pasting the link back brings it back.
- The browser never talks to Silo: it goes through `GET /api/files/{id}`, protected by the session. Silo's console is on http://localhost:9001 (credentials `S3_ACCESS_KEY` / `S3_SECRET_KEY`).

### Scripts

`search`, `recent`, `trash` and `show folder` accept `--json`, as does `show tag --list`, which also accepts `--yaml`:

```sh
notes search recette --json | jq -r '.[].id'
```

The detail of each command is in `notes <command> --help`.

## MCP (agents)

```sh
notes mcp   # speaks MCP on stdin/stdout, on behalf of the CLI session (notes login)
```

`notes mcp` gives an agent twenty-three tools on the account's notes: reading (`search_notes`, `recent_notes`, `read_note`, `list_folder`, `tags`, `note_history`, `note_links`, `read_file`, `export_notes`), writing (`create_note`, `edit_note`, `create_folder`, `rename_folder`, `move_item`, `rename_tag`, `replace_notes`, `delete_tag`, `attach_file`, `import_notes`) and trash (`delete_note`, `delete_folder`, `list_trash`, `restore_item`). A client of the API like the rest of the CLI, it listens on no port and has nothing to authenticate: it is the machine's CLI session that decides the server and the account, with all its rights.

An MCP is not read like an API: you do not run it by hand, the client starts it, and the account question is not settled in the request but by `notes login`. Everything is in **[MCP.md](MCP.md)**: declaring the server in the client, the account it acts for and how to share it between machines, the twenty-three tools, and what to know before letting an agent loose on your notes.

For an agent that cannot run a command (the Claude app on a phone calls from Anthropic's cloud), `notes-server serve --mcp-addr :8081` serves the same tools over streamable HTTP at `/mcp`, on an address of its own that a tunnel (Tailscale Funnel, Cloudflare Tunnel) or a proxy opens to the Internet without the API. Each request carries `Authorization: Bearer notes_…`, an API token of the account; a session token, an expired or revoked one gets 401. The tools carry `readOnlyHint`, `destructiveHint` and `idempotentHint`, so the client asks before deleting or overwriting. Setup in [MCP.md](MCP.md#4-depuis-le-téléphone--le-mcp-en-http).

## API

```sh
make api                                     # in Docker, with WEB_ORIGIN set in .env
go run ./cmd/notes-server serve --origin http://localhost:3000   # or directly on the machine
```

`--addr` chooses the listening address (`127.0.0.1:8080` by default) and `--origin` the allowed origin of the web app, `--web` a folder of the generated app (`bun run generate`) served at the same address as the API. `--add-user-mode` decides who creates an account from the app: `invitation` (default, by an admin's link) or `public` (anyone). On a database with no account, `NOTES_HOST_NAME` and `NOTES_HOST_PASSWORD` create the first one at startup. When the API is only reachable from the machine (`127.0.0.1`), it takes each device's address from the proxy's `X-Forwarded-For` (`tailscale serve`), so that the delay after wrong passwords only blocks the faulty device. In a container, `--published` gives the address published on the machine side (`NOTES_LISTEN`), instead of `--addr`. `--mcp-addr` (none by default) also serves the MCP tools over HTTP at `/mcp` on that address, for agents holding an API token (see *MCP*).

| Resource | Routes (prefix `/api`) | JSON body |
|---|---|---|
| Accounts (admin) | `GET`, `DELETE /users/{id}`, `GET`, `POST /invites`, `DELETE /invites/{id}` | |
| Account creation | `GET /add-user[?invite=<token>]`, `POST /signup` | `name`, `password`, `invite` (the link's token) |
| Account | `POST /login`, `POST /logout`, `GET`, `PATCH /me`, `PATCH /me/password` | `name`, `password` (login); settings: `locale`, `theme`, `color_mode`, `view`, `trash_retention_days` (0 to 3650, 0 to never purge), `name`, `avatar` (id of an image of the account, `""` for none); password: `current`, `new` |
| Notes | `POST /notes`, `GET`, `PATCH`, `DELETE /notes/{id}`, `GET /notes/{id}/neighbors`, `GET /pinned` | `content`, `tags`, `parent_id` (creation), `pinned`, `readonly` |
| Folders | `GET`, `POST /folders`, `GET`, `PATCH`, `DELETE /folders/{id}` | `title`, `parent_id` (creation), `pinned` |
| Browse | `GET /items?parent=<uuid>` (root without `parent`), `POST /move` | `ids`, `parent_id` (root without `parent_id`) |
| Tags | `GET`, `POST /tags`, `GET`, `PATCH`, `DELETE /tags/{value}` | `values` (creation), `value` (rename) |
| Graph | `GET /graph` (tags counted), `GET /graph/notes` (notes and the tags that link them, laid out; `?layout=false` without the layout) | |
| Search | `GET /search?q=<text>`, `GET /recent` | |
| Trash | `GET`, `DELETE /trash` (everything), `DELETE /trash/{id}` (one deletion and what left with it), `POST /restore` | `ids` (restore) |
| Export / import | `GET /export` (zip; `?files=false` leaves the images of `_files/` empty, only named), `POST /import?parent=<uuid>` (zip as body, `Content-Type: application/zip`, or `multipart/form-data` form of `files` named by their path, `folder/note.md`; root without `parent`) | |
| Images and attachments | `POST /files?name=<name>` (the file as body), `GET /files/{id}` | |

- Everything, except `POST /login`, `GET /add-user` and `POST /signup`, requires a session, otherwise 401. The `/users` and `/invites` routes also require an admin account, otherwise 403. `POST /signup` is only open on a database with no account, with a valid invitation token, or in `public` mode, where the same address waits longer and longer after each account created (429, beyond four); the token works once and expires after 7 days, the database only keeps its fingerprint. Deleting an account erases its notes, folders, tags and images (bytes included), but neither the logged-in account nor the last admin. Login sets a `notes_session` cookie (`HttpOnly`, `Secure`, `SameSite=Strict`), which lasts 30 days extended at each request. Scripts can send the token in `Authorization: Bearer <token>`. Expired sessions are erased from the database at the server's hourly purge.
- After consecutive wrong passwords (per address for `POST /login`, per account for `PATCH /me/password`), the route answers 429 with `Retry-After` (in seconds): nothing up to 3 failures, then 30 s, 1 min, 5 min, 15 min, and 1 h from 8. A success resets the counter. The counts are kept in the database (`throttles`), so a restart forgets none; a day after the last try, an address starts over.
- The title of a note comes from its content, as in the CLI: the API refuses a `title` field for a note.
- `PATCH /notes/{id}` only changes the fields sent. `{"pinned": true}` alone pins the note without touching its text or its modification date. Without `tags`, the tags are the `#words` of the content. With `tags`, this list rewrites the content: `"tags": []` erases all its `#words`. It answers the complete note, like `GET` (saved text, tags, path). `{"readonly": true}` locks the note: `content` and `tags` are then refused (409), unless with `"readonly": false` in the same request. With `updated_at` (the one read by `GET`, or `created_at` for a note never modified), `content` and `tags` are refused (409) if the note has changed since: a device left open does not overwrite what another wrote.
- `GET /notes/{id}/versions` lists the note's versions, from the most recent to the oldest, without their text (`id`, `conflict`, `created_at`); `GET /notes/{id}/versions/{version}` reads one with its `content`. A `PATCH` (or `notes edit note`) that changes the text keeps the old one at the first save of a writing session (the note not modified for 15 minutes), and at least once an hour in a long session; a save without change does not count. Versions are not limited in number. Restoring a version is a `PATCH` of its `content`; versions go with the note when it is deleted for good.
- `POST /notes/{id}/versions` with `{"content": "..."}` keeps a text refused by a 409 as a version marked `conflict` (201), so as not to lose it; nothing if it is already the note's text (204).
- `PATCH /folders/{id}` with `{"pinned": true}` makes the folder a favorite. `GET /pinned` lists the pinned notes and the favorite folders, with their `kind`.
- Errors have the form `{"error": "..."}`, with status 400, 401, 403, 404, 409, 413, 415, 429 or 500.
  Only an unknown route (404) and an unhandled method (405) answer in plain text, because the router writes them.
- `PATCH /me` with `name` renames the account (409 if the name is taken). `PATCH /me/password` checks `current`, then logs out the other devices: the one that sends it stays logged in. The image of `avatar` is not purged as long as it serves as an avatar.
- `DELETE /me/data` erases for good all the account's notes, folders, tags and images, trash included. The account, its settings and its sessions stay. It is the "Danger zone" of the web app's settings.
- Deleted items go through the trash. The server purges it every hour. `DELETE /trash` empties it right away and for good.
- `GET /export` and `POST /import` work like `notes export` and `notes import`, as a zip; the import also takes a form of files, their paths giving the folders. The import goes into the `parent` folder, at the root without it. It answers `{"folders": n, "notes": n}`, or 409 if a folder of the zip already exists at that spot, 400 if two files of the form have the same path.
- `POST /files` answers the created file (`id`, `name`, `content_type`, `size`), 415 if it is not an accepted image. `GET /files/{id}` returns the image, cached by the browser: an image never changes.
- `make openapi` generates the OpenAPI contract in `docs/swagger.yaml`, from the `@` comments of the routes.

### Security

Every request must come from a logged-in account. Still keep the API on `127.0.0.1` behind Tailscale or an HTTPS proxy: the password and the cookie must never travel in clear over a network.

- A request whose `Origin` header differs from `--origin` is refused (403). Without `--origin`, no web page can call it. The `SameSite=Strict` cookie adds protection against other sites.
- A request without `Origin` (`curl`, scripts) is accepted if it carries a session.
- The `Host` header must be `localhost`, `127.0.0.1`, `[::1]` or the host of `--origin`, which blocks DNS rebinding.

### Encryption

The text and titles of notes, the names of folders, the tags, the snippets and the names of images are encrypted in the database, and the images in Silo (AES-256-GCM). Each account has its own key, drawn at random at its first login and stored in `users.data_key`, itself encrypted by the server secret `NOTES_SECRET`. A stolen database, backup or disk stays unreadable without this secret. A compromised server, on the other hand, holds it: encryption does not protect against that.

- Generate the key with `openssl rand -base64 32` and put the same one in `.env` (`notes-server` run by `go run`, the tests) and `.env.docker` (API).
- **Keep a copy of the key in a password manager, never in a backup.** If you lose it, the notes become unreadable, even with a backup.
- A forgotten password loses nothing: the admin resets it in Settings > Users (or `notes-server user reset-password` on the server, the only recourse for their own account), since the keys do not depend on the password.
- Changing the secret: `make backup`, put the new one in `NOTES_SECRET` (`.env`), then `NOTES_OLD_SECRET=<old> notes-server rotate-secret` (in the container: `docker exec -e NOTES_OLD_SECRET=<old> -e NOTES_SECRET=<new> note-api /notes-server rotate-secret`). Only the accounts' keys are re-encrypted, not the notes. Then put the new secret in `.env.docker` and run `make api`. A second pass does nothing.
- `notes export` outputs clear text.
- To find a tag, a folder or a note by its name, and keep those names unique, the database keeps an HMAC fingerprint of the lowercase name (`title_hash`, `value_hash`). It cannot be read, but two identical names have the same fingerprint. Paths, sorting and search are done in the API, after decryption.
- Not encrypted: the dates and the tree (which item is in which folder).

## Configuration

These variables are those of `notes-server`; the CLI needs none of them, its session comes from `notes login`.

| Variable | Role | Default |
|---|---|---|
| `POSTGRES_USER`, `POSTGRES_PASSWORD` | Database credentials | |
| `POSTGRES_DB` | Database name | `notes` |
| `POSTGRES_HOST` | Host | `localhost` |
| `PORT` | PostgreSQL port | `5432` |
| `CONFIG_FILE` | Replaces the embedded configuration | `config/config.yaml`, built into the binary |
| `WEB_ORIGIN` | Origin of the web app for `make api` | none |
| `S3_ENDPOINT` | Address of Silo, without `http://` (`note-silo:9000` in `.env.docker`) | |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | Silo credentials, which also create it at the first `make up` | |
| `S3_BUCKET` | Bucket for images, created at first launch | |
| `S3_SECURE` | `true` to talk to Silo over HTTPS | no |
| `NOTES_SECRET` | Encryption key of the notes, 32 bytes in base64 (see Encryption) | none: required to write |

The commands, their help texts and their flags are defined in `config/config.yaml`. This file is built in at compile time: rebuild after changing it, or point `CONFIG_FILE` to your file.

## Backup

```sh
make backup    # writes backups/notes-<date>.sql and the images in backups/notes-<date>.files/ (ignored by git)
```

To restore:

```sh
docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"' < backups/<file>.sql
docker compose cp backups/<file>.files silo:/tmp/restore
docker compose exec silo sh -c 'mcli alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" && mcli mirror /tmp/restore local/notes'
```

To erase everything, all accounts combined, after a backup: `make purge` (you have to type `purge` to confirm). The accounts and their settings stay.

The dump starts by dropping the existing tables: restoring therefore replaces the whole content of the database. The images are added to those in Silo, without erasing anything.

## Development

```sh
make setup    # installs the lefthook hooks
make test     # all the tests, PostgreSQL must be running
make test ARGS="./app/shared/services/process/..."
make down     # stops the containers and Colima
```

The tests run in a separate database, `<POSTGRES_DB>_test`, created at the first `make test`, each in a rolled-back transaction. They create the schema: a migration being written therefore never reaches the database the API serves.

LibreDB Studio, an interface to explore the database, is on <http://localhost:3001>. On the command line: `make pgsql` opens psql, `make pgsql QUERY="SELECT ..."` runs a query and stops at the first error.

| Folder | Content |
|---|---|
| `cmd/` | Declaration of the Cobra commands |
| `app/cli/` | Entry point and display of each command |
| `app/shared/services/process/` | Business logic and queries, reusable by the API |
| `app/shared/domain/` | Data models |
| `app/api/` | API routes, one per folder |
| `config/` | Configuration and command texts |

## License

[MIT](LICENSE)
