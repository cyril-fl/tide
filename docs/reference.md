# Tide reference

The exact rules behind Tide, for those who host it, script it or work on it. The [README](../README.md) is the short tour; the [wiki](https://github.com/cyril-fl/tide/wiki) shows the web app.

- [Accounts](#accounts)
- [Notes, titles and tags](#notes-titles-and-tags)
- [Links between notes](#links-between-notes)
- [Trash](#trash)
- [Export and import](#export-and-import)
- [Images and attachments](#images-and-attachments)
- [CLI sessions and API tokens](#cli-sessions-and-api-tokens)
- [MCP over HTTP](#mcp-over-http)
- [API](#api)
- [Security](#security)
- [Encryption](#encryption)

## Accounts

Each note, folder and tag belongs to an account, and an account never sees those of the others.

The first account in the database is admin. Only it invites, lists and deletes the others, from the app's settings (*Users* section). This first account is created in three ways:

- on an empty database, the app leads straight to the account creation page;
- if `TIDE_HOST_NAME` and `TIDE_HOST_PASSWORD` are set in the `.env`, Tide creates it at startup; those two lines should then be emptied;
- `tide-server user add <name>` works in every case, whatever the server's mode.

After that, `TIDE_ADD_USER_MODE` decides who can create an account:

- `invitation` (default): the admin creates an invitation link in the settings and sends it. The link works once and expires after 7 days. The invitee chooses their own name and password. Without an invitation, the account creation page is closed.
- `public`: anyone who can reach the app can create an account. Reserve this mode for a trusted network (Tailscale), since nothing limits the number of creations.

The admin commands of `tide-server`, run in the container (`docker exec -it tide-api /tide-server …`, `-i` alone with `--password-stdin`):

```sh
tide-server user add cyril              # asks for the password twice (8 characters at least)
tide-server user reset-password cyril   # forgotten password: new password, all devices logged out
tide-server rotate-secret               # re-encrypts the accounts' keys with a new TIDE_SECRET (see Encryption)
```

- `--password-stdin` reads the password from standard input, for `user add` and `user reset-password`.
- The admin flag is set on the first account created in an empty database, however it is created. To move it or add one, you have to touch the database: `UPDATE users SET admin = true WHERE name = '<name>'`.
- Deleting an account permanently erases its notes, folders, tags and images. An admin can neither delete themselves nor delete the last admin.
- `tide account purge` empties the logged-in account (notes, folders, tags, images), after typing its name. The account and its settings stay.

## Notes, titles and tags

**Titles.** A note has no separate title: it is its first markdown heading (`#`, `##`…, whatever the level), otherwise the first 50 characters of its first line of text, without formatting, code or lines made only of `#tags`. A note without text stays untitled. The title follows the content at each change, and the path of a note (`/Projets/Idées`) with it. The API refuses a `title` field for a note.

**Paths.** The `note` and `folder` commands, as well as `move`, `finder` and `graph`, accept a UUID or a path (`/Projets/Go/Idées`). A path must match exactly, capitals included. If several items have the same path, the error lists their UUIDs.

**Folder names.** Two folders in the same folder cannot have the same name, capitals included: creating, renaming, moving or restoring a folder next to a namesake is refused (409 in the API). A database created before this rule has its duplicates renamed at startup, `Name (2)`, `Name (3)`… Notes, on the other hand, can share a title.

**Tags.** The tags of a note are the `#words` of its text, code and links excluded: `#idée` in the content creates the tag and links it to the note, and erasing it removes it. `--tag` writes `#tag` at the end of the text and `--untag` erases it. Renaming or deleting a tag fixes the text of all its notes, and a note that ends up empty goes to the trash. A tag with no note left disappears. A tag contains letters, digits, `_` or `-`, with at least one letter.

**Forms.** If a value is missing on the command line, a form opens. Without a terminal (cron, CI), pass everything through flags.

## Links between notes

Write `[[Title]]` in a note to link it to the note with that title, capitals or not, outside code. On save, a `[[Title]]` that designates a single note becomes `[Title](note:<uuid>)`: the link now follows that note by its id, even if another takes its title, and the text between brackets updates when it changes title. If no note, or several, have that title, `[[Title]]` waits: it will be linked as soon as a single note bears that title. To break a tie, write its path, `[[/Projets/Idées]]`.

`[[Title#Section]]` targets a heading of the note: it becomes `[Title#Section](note:<uuid>#Section)`, keeps the section when the note changes title, and becomes `[[Title#Section]]` again on export. In the app, typing `#` after a note's name offers its headings, and a click opens the note at that heading. `[[#Section]]` targets a heading of the note itself: it stays as is, `[[` then `#` offers the note's headings, and a click scrolls to it.

`![[Title]]`, like in Obsidian, includes the note: it becomes `![Title](note:<uuid>)` like a link, and becomes `![[Title]]` again on export. In the app, the included note is displayed read-only under a link that opens it; its own inclusions stay links, so that a note that includes itself, or includes a note that includes it, stops there.

`GET /api/notes/{id}/neighbors` gives its links (`links`) and its backlinks (`backlinks`), and the graph also connects linked notes. In the app, typing `[[` offers the notes, and a click on a link opens it. `GET /api/links/broken` lists the links of live notes that lead nowhere: `unresolved` (no other note with that title or path, or a `#Section` link to a heading the note does not have), `ambiguous` (several), `missing` (link to a note put in the trash or purged) or `file` (image or attachment `/api/files/<id>` that the account does not have: imported from another account, purged).

## Trash

Deleted items go through the trash. `tide restore` only accepts UUIDs, displayed by `tide trash`. Items are deleted for good once the account's retention period has passed (30 days by default, adjustable in Settings > Trash of the app, "Never erased" to not purge): the server purges every hour, and `DELETE /trash` empties it right away.

## Export and import

`export` writes into a new or empty folder, never over files: a folder becomes a directory, a note a `<title>.md` file with its text as is (title and `#tags` included). `[Title](note:<uuid>)` links become `[[Title]]` again, since ids mean nothing outside this database. Two notes with the same title in the same folder give `Title.md` and `Title (2).md`.

`import` does the opposite, all or nothing: a folder name already taken at the same place stops the whole import, leaving nothing behind. Hidden files (`.obsidian`, `.git`…) and those that are not `.md` are ignored. A text without a markdown title gets its file name at the top, `# Name`, like in Obsidian. `[[links]]` are linked at import, as when typing.

Images follow the notes:

- On export, the images a note cites are copied into `_files/` at the root of the export, and the note cites them by a relative path (`../_files/<uuid>.png`). An attachment keeps the extension of its name (`.docx`, `.xlsx`…).
- On import, an image cited by a relative path (`![](photo.png)`, `![](<photo de mer.png>)`) becomes an image of the note, wherever it is in the folder. `_files/` does not become a folder. An image that is missing or not PNG, JPEG, GIF or WebP keeps its link as is.
- A link to a file of the folder (`[rapport](rapport.docx)`) or a `![[son.mp3]]` that is not an image makes it an attachment, cited by a link; a link to a note or to a refused type stays as is.
- Obsidian's `![[image.png]]` and `![[image.png|300]]` are imported too: the image is looked up by name in the whole folder (the one in the note's folder first), and the width becomes the closest editor size (`"small"` up to 300 px, `"medium"` up to 600, nothing beyond).

## Images and attachments

Pasting or dragging an image into the app's editor sends it to the API, which stores it in [Silo](https://github.com/pgsty/silo) (an S3 storage, a MinIO fork) and describes it in the `files` table. The note cites it in markdown: `![name](/api/files/<uuid>)`. Another file (PDF, zip, hence also Office documents, MP3, WAV, Ogg, MP4, WebM) becomes an attachment, cited by a link `[name](/api/files/<uuid>)`: the API sends it as a download under its name, and it follows the same purge and export rules as an image. The avatar stays an image. `GET /api/files` lists the account's files (name, type, size), the heaviest first, without those of a purged note or erased data.

- PNG, JPEG, GIF and WebP only (no SVG, which can contain scripts), 20 MB at most for any file. The type is read from the file, not from what the browser sends.
- An image follows its note into the trash: when the note is erased for good (period elapsed, trash emptied, or its folder purged), its images go with it at the next hourly purge, unless another note still cites them.
- An image removed from a note's text waits for the account's trash period, counted from when it is no longer cited (never if the trash keeps everything): until then, undoing or pasting the link back brings it back.
- The browser never talks to Silo: it goes through `GET /api/files/{id}`, protected by the session. Silo's console is on http://localhost:9001 (credentials `S3_ACCESS_KEY` / `S3_SECRET_KEY`).

## CLI sessions and API tokens

**CLI session.** `tide login <url>` keeps the server URL and the token in `~/.config/tide/session.json`, readable by the user only. The session lasts 30 days, extended at each use. `TIDE_URL` and `TIDE_SESSION`, if set, take precedence over this file: `tide login <url> --print-token` creates a separate session and prints only its token, for an agent that cannot log in where it runs (see [`MCP.md`](../MCP.md)). `tide logout` with the variable only ends that session. The token opens the whole account: it goes in no versioned file.

**API tokens.** An API token (`notes_…`, from `tide add token` or Settings > Account) is sent as `Authorization: Bearer <token>`. It does everything the account does on notes, folders, tags and files, but does not manage the account: its tokens, its password, its settings, the purge, the logout and the admin routes answer 403. A `--read-only` token reads and changes nothing. The database only keeps its fingerprint (SHA-256). Once expired, it is refused (401) then purged by the server's hourly cleanup.

## MCP over HTTP

`tide mcp` speaks MCP on stdin/stdout for agents that can run a command. For an agent that cannot (the Claude app on a phone, or ChatGPT calling from their cloud), `tide-server serve --mcp-addr :8081` serves the same tools over streamable HTTP at `/mcp`, on an address of its own that a tunnel (Tailscale Funnel, Cloudflare Tunnel) or a proxy opens to the Internet without the API.

- Each request carries `Authorization: Bearer notes_…`: an API token of the account, or the access token of an OAuth connection. A session token, an expired or revoked one gets 401.
- Tide is the OAuth 2.1 authorization server of that address (metadata, open dynamic client registration, a sign-in and consent page, PKCE, refresh tokens rotated), so ChatGPT, Claude's "Connect" or any MCP client that signs in needs only the URL. Each connection is listed and revoked with the API tokens.
- A read-only token or connection gets the reading tools only, and the API refuses it any change.
- The tools carry `readOnlyHint`, `destructiveHint` and `idempotentHint`, so the client asks before deleting or overwriting.

Setup in [MCP.md](../MCP.md#4-depuis-le-téléphone-ou-chatgpt--le-mcp-en-http).

## API

```sh
make api                                                         # in Docker, with WEB_ORIGIN set in .env
go run ./cmd/tide-server serve --origin http://localhost:3000    # or directly on the machine
```

The flags of `tide-server serve`:

| Flag | Role |
|---|---|
| `--addr` | Listening address, `127.0.0.1:8080` by default |
| `--origin` | The allowed origin of the web app |
| `--web` | A folder of the generated app (`bun run generate`), served at the same address as the API |
| `--add-user-mode` | Who creates an account from the app: `invitation` (default) or `public` |
| `--published` | In a container, the address published on the machine side (`TIDE_LISTEN`), instead of `--addr` |
| `--mcp-addr` | Also serves the MCP tools over HTTP at `/mcp` on that address, with its OAuth (none by default) |
| `--mcp-published` | The MCP's address on the machine side (`TIDE_MCP_LISTEN`) |

When the API (or the MCP) is only reachable from the machine (`127.0.0.1`), it takes each device's address from the proxy's `X-Forwarded-For` (`tailscale serve`), so that the delay after wrong passwords only blocks the faulty device.

`make openapi` generates the full OpenAPI contract in [`docs/swagger.yaml`](./swagger.yaml), from the `@` comments of the routes. The routes, prefix `/api`:

| Resource | Routes | JSON body |
|---|---|---|
| Accounts (admin) | `GET`, `DELETE /users/{id}`, `GET`, `POST /invites`, `DELETE /invites/{id}` | |
| Account creation | `GET /add-user[?invite=<token>]`, `POST /signup` | `name`, `password`, `invite` (the link's token) |
| Account | `POST /login`, `POST /logout`, `GET`, `PATCH /me`, `PATCH /me/password`, `DELETE /me/data` | `name`, `password` (login); settings: `locale`, `theme`, `color_mode`, `view`, `trash_retention_days` (0 to 3650, 0 to never purge), `name`, `avatar` (id of an image of the account, `""` for none); password: `current`, `new` |
| Notes | `POST /notes`, `GET`, `PATCH`, `DELETE /notes/{id}`, `GET /notes/{id}/neighbors`, `GET /pinned` | `content`, `tags`, `parent_id` (creation), `pinned`, `readonly` |
| Versions | `GET`, `POST /notes/{id}/versions`, `GET /notes/{id}/versions/{version}` | `content` |
| Folders | `GET`, `POST /folders`, `GET`, `PATCH`, `DELETE /folders/{id}` | `title`, `parent_id` (creation), `pinned` |
| Browse | `GET /items?parent=<uuid>` (root without `parent`), `POST /move` | `ids`, `parent_id` (root without `parent_id`) |
| Tags | `GET`, `POST /tags`, `GET`, `PATCH`, `DELETE /tags/{value}` | `values` (creation), `value` (rename) |
| Graph | `GET /graph` (tags counted), `GET /graph/notes` (notes and the tags that link them, laid out; `?layout=false` without the layout) | |
| Search | `GET /search?q=<text>`, `GET /recent` | |
| Links | `GET /links/broken` | |
| Trash | `GET`, `DELETE /trash` (everything), `DELETE /trash/{id}` (one deletion and what left with it), `POST /restore` | `ids` (restore) |
| Export / import | `GET /export` (zip; `?files=false` leaves the images of `_files/` empty, only named), `POST /import?parent=<uuid>` (zip as body, `Content-Type: application/zip`, or `multipart/form-data` form of `files` named by their path, `folder/note.md`; root without `parent`) | |
| Images and attachments | `GET /files`, `POST /files?name=<name>` (the file as body), `GET /files/{id}` | |

### Sessions and errors

- Everything, except `POST /login`, `GET /add-user` and `POST /signup`, requires a session, otherwise 401. The `/users` and `/invites` routes also require an admin account, otherwise 403.
- Login sets a `tide_session` cookie (`HttpOnly`, `Secure`, `SameSite=Strict`), which lasts 30 days extended at each request. Scripts can send the token in `Authorization: Bearer <token>`. Expired sessions are erased at the server's hourly purge.
- `POST /signup` is only open on a database with no account, with a valid invitation token, or in `public` mode, where the same address waits longer and longer after each account created (429, beyond four). The token works once and expires after 7 days; the database only keeps its fingerprint.
- After consecutive wrong passwords (per address for `POST /login`, per account for `PATCH /me/password`), the route answers 429 with `Retry-After` (in seconds): nothing up to 3 failures, then 30 s, 1 min, 5 min, 15 min, and 1 h from 8. A success resets the counter. The counts are kept in the database (`throttles`), so a restart forgets none, and the OAuth consent page of the MCP address shares the count of an address with the login; a day after the last try, an address starts over.
- Errors have the form `{"error": "..."}`, with status 400, 401, 403, 404, 409, 413, 415, 429 or 500. Only an unknown route (404) and an unhandled method (405) answer in plain text, because the router writes them.

### Notes and versions

- `PATCH /notes/{id}` only changes the fields sent. `{"pinned": true}` alone pins the note without touching its text or its modification date. It answers the complete note, like `GET` (saved text, tags, path).
- Without `tags`, the tags are the `#words` of the content. With `tags`, this list rewrites the content: `"tags": []` erases all its `#words`.
- `{"readonly": true}` locks the note: `content` and `tags` are then refused (409), unless with `"readonly": false` in the same request.
- With `updated_at` (the one read by `GET`, or `created_at` for a note never modified), `content` and `tags` are refused (409) if the note has changed since: a device left open does not overwrite what another wrote.
- `GET /notes/{id}/versions` lists the note's versions, from the most recent to the oldest, without their text (`id`, `conflict`, `created_at`); `GET /notes/{id}/versions/{version}` reads one with its `content`. A change of text keeps the old one at the first save of a writing session (the note not modified for 15 minutes), and at least once an hour in a long session; a save without change does not count. Versions are not limited in number. Restoring a version is a `PATCH` of its `content`; versions go with the note when it is deleted for good.
- `POST /notes/{id}/versions` with `{"content": "..."}` keeps a text refused by a 409 as a version marked `conflict` (201), so as not to lose it; nothing if it is already the note's text (204).

### The rest

- `PATCH /folders/{id}` with `{"pinned": true}` makes the folder a favorite. `GET /pinned` lists the pinned notes and the favorite folders, with their `kind`.
- `PATCH /me` with `name` renames the account (409 if the name is taken). `PATCH /me/password` checks `current`, then logs out the other devices: the one that sends it stays logged in. The image of `avatar` is not purged as long as it serves as an avatar.
- `DELETE /me/data` erases for good all the account's notes, folders, tags and images, trash included. The account, its settings and its sessions stay. It is the "Danger zone" of the web app's settings.
- `GET /export` and `POST /import` work like `tide export` and `tide import`, as a zip; the import also takes a form of files, their paths giving the folders. The import goes into the `parent` folder, at the root without it. It answers `{"folders": n, "notes": n}`, or 409 if a folder of the zip already exists at that spot, 400 if two files of the form have the same path.
- `POST /files` answers the created file (`id`, `name`, `content_type`, `size`), 415 if its type is not accepted. `GET /files/{id}` returns the file, cached by the browser: a file never changes.

## Security

Every request must come from a logged-in account. Still keep the API on `127.0.0.1` behind Tailscale or an HTTPS proxy: the password and the cookie must never travel in clear over a network. The session cookie requires HTTPS, which the proxy provides.

- A request whose `Origin` header differs from `--origin` (`WEB_ORIGIN`, the exact address the browser types) is refused (403). Without `--origin`, no web page can call it. The `SameSite=Strict` cookie adds protection against other sites.
- A request without `Origin` (`curl`, scripts) is accepted if it carries a session.
- The `Host` header must be `localhost`, `127.0.0.1`, `[::1]` or the host of `--origin`, which blocks DNS rebinding.
- The MCP address only serves `/mcp` and its OAuth sign-in, a token or a sign-in required, so a tunnel can open it to the Internet while the rest stays private.

## Encryption

The text and titles of notes, the names of folders, the tags, the snippets and the names of images are encrypted in the database, and the images in Silo (AES-256-GCM). Each account has its own key, drawn at random at its first login and stored in `users.data_key`, itself encrypted by the server secret `TIDE_SECRET`. A stolen database, backup or disk stays unreadable without this secret. A compromised server, on the other hand, holds it: encryption does not protect against that.

- Generate the key with `openssl rand -base64 32` and put the same one in `.env` (`tide-server` run by `go run`, the tests) and `.env.docker` (API).
- **Keep a copy of the key in a password manager, never in a backup.** If you lose it, the notes become unreadable, even with a backup.
- A forgotten password loses nothing: the admin resets it in Settings > Users (or `tide-server user reset-password` on the server, the only recourse for their own account), since the keys do not depend on the password.
- Changing the secret: `make backup`, put the new one in `TIDE_SECRET` (`.env`), then `TIDE_OLD_SECRET=<old> tide-server rotate-secret` (in the container: `docker exec -e TIDE_OLD_SECRET=<old> -e TIDE_SECRET=<new> note-api /tide-server rotate-secret`). Only the accounts' keys are re-encrypted, not the notes. Then put the new secret in `.env.docker` and run `make api`. A second pass does nothing.
- `tide export` outputs clear text.
- To find a tag, a folder or a note by its name, and keep those names unique, the database keeps an HMAC fingerprint of the lowercase name (`title_hash`, `value_hash`). It cannot be read, but two identical names have the same fingerprint. Paths, sorting and search are done in the API, after decryption.
- Not encrypted: the dates and the tree (which item is in which folder).
