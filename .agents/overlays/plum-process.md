Notes (tide).

- Processes in `core/services/process/<verb><noun>/` (e.g. `createtag`, `createschema`, `listtag`). They take `ctx` and a `bun.IDB`, so tests pass a transaction.
- Every query filters by the account: `domain.UserID(ctx)`.
- Sentinel errors that the API must answer as 400 go into the list of `core/api/services/respond/respond.go`.
