---
name: notes-cli-command
description: Add or change a Tide CLI command (add/show/edit/remove on a noun, or a verb of its own) end to end - config.yaml entry and config.go key, cobra wiring, the command as a client of the API through apiclient, the same capability on the MCP, --json output, tests against the test server, README.
---

# Tide CLI command

`tide` is a client of the API: a command never opens the base, it calls a route through `cli/services/apiclient`. Its MCP (`cli/cmd-mcp/`) is the other face of the same client, and the two move together (`cli/CLAUDE.md`). Reference implementations: `cli/cmd-read-search` (list, run, --json), `cli/cmd-create-search` + `cli/cmd-edit-search` (flags shared, only the given ones changed), `cli/cmd-read-note` (one or many args, --json object or array).

## 1. Ask before writing

Ask the user (AskUserQuestion, recommended option first), with the command lines as a preview:

- **Name**: the verbs are `add`, `show`, `edit`, `remove` on a noun (`cmd/<verb>.go`, config key `<create|read|edit|delete>-<noun>`), or a verb of its own (`replace`, `search`, `trash`).
- **Input**: positional args (one, many, optional) or flags. Notes and folders are named by uuid or path; other things by name or id.
- **Edge cases**: not found, already exists, refused by the plan, an empty result (`[]` in --json, a message in plain text).

## 2. Where things go

| What | Where |
|------|-------|
| Name, help, examples, flags | `config/config.yaml`, `commands.<key>` |
| The key in the config struct | `config/config.go`, `commands` (a key missing there gives an empty `Use`) |
| Cobra wiring: `Args`, flags, mutually exclusive | `cmd/<verb>.go` |
| The command | `cli/cmd-<key>/cmd.go` |
| Code the MCP needs too | `cli/services/apiclient/` (decoding, account edits, saved searches, snippets, files, batched reads already there) |
| A route the API lacks | `core/api/` (`core/api/CLAUDE.md`) on a plum Process (`plum-process` skill), then `make openapi` and `bun run types` in `web/` |

Wiring helpers of `cmd/resolve.go`: `apiResolved(kind, run)` turns paths in the args and `--parent` into uuids and `--content -` into stdin; `withStdin(run)` only does stdin, for args that are names.

## 3. The command

```go
// Read lists the snippets, or prints the markdown of the one named.
func Read(cobra *cobra.Command, args []string) error {
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	snippets, err := apiclient.Snippets(cobra.Context(), client)
	// ...
}
```

- Call a route with `apiclient.Decode[T](client.GetApiX(ctx, ...))`, or `Paged[T]` for a list with its `X-Total-Count`; a non-2xx answer is the API's error, returned as is.
- `apiclient.Long(ctx)` for an upload, a download or an export.
- Output: plain text by default (`style.Table`, `style.Empty` for nothing, `Successfully ...` for a write), `--json` on every command that reads, with `[]` rather than null. Messages in English.
- Edit commands change only what a flag asks for: `flags.Changed(name)` decides.
- An interactive form for what is missing is a mill (`services/mill/`, see `cmd-create-note`); a new command may require its flags instead (`MarkFlagRequired`).

## 4. The MCP side, in the same change

What the command adds, the MCP gets too, and the reverse: an option or a field folded into an existing tool (few tools, one-sentence descriptions), on the same `apiclient` function. Exempt: screens (`finder`, `graph`), session and account commands, forms. A gap left on purpose gets a `TODO:` and an issue.

## 5. Tests

Each command gets a test in its package against the test server, no mock:

```go
ctx, tx := apitest.Serve(t) // an account with every plan, the API on a transaction rolled back after
command := &cobra.Command{}
plumcliutilsargs.Makeflags(command, config.Current.Commands.ReadSnippet.Flags) // the real flags: catches a rename
if err := command.ParseFlags([]string{"--json"}); err != nil {
	t.Fatal(err)
}
command.SetContext(ctx)
printed, err := apitest.Stdout(t, func() error { return Read(command, nil) })
```

- Seed with the processes of `core/services/process/` on `tx` (`createnote`, `createfolder`, `deletetag`...).
- A plan refusal: `apitest.Plan(t, ctx, tx, "base")`, last in the test.
- A command that writes: check the write by reading it back through another command or a process.
- Run: `make test ARGS="./cli/... ./cmd/... ./config/..."` (the stack up). Shared code changed: `./cli/cmd-mcp/` too.

## 6. By hand, then the README

- `tide whoami` first: the local CLI session may be the user's real server, not the dev stack. There, only reads and `--dry-run`; writes are proven by the tests.
- `README.md`, "Use it from the terminal": one line of the tour for a new command or option. `MCP.md` (French) when a tool changes.
