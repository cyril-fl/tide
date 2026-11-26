Notes (tide), Postgres and Silo.

- Run with `make test` only, never `go test` directly, and never bypass the Makefile to make a test pass. Target with `ARGS`: `make test ARGS="./core/services/process/createtag -run TestX"`. Needs the stack up (`make up`); each run gets a base and a bucket of its own, so agents can run it at once.
- When the host port 5432 is not the stack's, run in the container (see the "Run" section of `CLAUDE.md`).
- Helpers: `testaccount` for a process with an account; `core/api/router_test.go` covers the API; CLI tests through `apitest.Serve(t)`.
- e2e (`web/test/e2e`): see `.claude/rules/e2e.md`.
