Notes (tide). Repo `cyril-fl/tide`, issues in French. Ignore `../deprecated/`.

## Load to judge against

One account, a phone and a tablet over Tailscale. In prod (`compose.prod.yaml`) one binary serves the API and the web app on `127.0.0.1:8080`, behind an HTTPS proxy: `r.RemoteAddr` is the proxy for every request.

## Where mistakes cost the most

1. **Auth** (`core/api/services/auth`, `guard`, `api/login/`, processes `createsession`, `readsession`, `editpassword`, `resetpassword`): every route but `POST /api/login` needs a session, the Origin check, the throttle keys.
2. **Accounts apart**: every query filters by `domain.UserID(ctx)`. Grep the processes for a `NewSelect`/`NewUpdate`/`NewDelete`/`NewRaw` without `user_id`.
3. **Data writes and deletes**: `deleteitems`, `restoreitems`, `moveitems`, `purgeitems`, `purgefiles`, `purgeaccount`, `emptytrash`, `importnotes`, `rotatesecret`. Look for missing transactions, wrong order, swallowed errors.
4. **Encryption** (`core/services/secret`): what is sealed, what stays in clear, the blind indexes.
5. **Limits**: body sizes (`respond.Decode` 10 MiB, files 20 MiB, import 200 MiB) against the server timeouts in `core/server/cmd-serve/cmd.go`.
6. **Background jobs**: the `every` loop of `tide-server serve` (it also runs once at start).
7. **The app** (`web/app`): save on close and conflicts in `NoteEditor.vue`, `createSaver`, any raw HTML.
8. **Tests**: risky code above with no test that fails when it breaks (`router_test.go` covers the API; `run_test.go` per process; e2e in `web/test/e2e`).

## May run

`go vet ./...` at the root. `make test` needs Postgres: only if `docker ps` shows `note-postgres`.
