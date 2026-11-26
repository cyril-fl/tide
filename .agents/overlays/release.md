Notes (tide).

- Repo `cyril-fl/tide`, branch `develop` (not the default: `Fixes #n` closes nothing, close by hand). Issues and comments in French; the subagent talks to the caller in English.
- Checks: `make test`, then `cd web && bun run test && bun run typecheck && bun run lint`; plus `make e2e` whenever a commit touches `web/` (CI runs every spec on each push: a deleted component broke six of them in 61a872e).
- Handoff: update the « Reprise » section of the plan file named in the roadmap memory (`~/.claude/projects/-Users-cyrilflambard-Devloppement-go-notes-develop/memory/roadmap-v0-12.md`), and that memory.
- A follow-up issue holds what is left, including a step on the user's side (an `ALTER` on the dev base or the server): it also goes in the closing comment.
- Never the wiki: the `tide-wiki` skill runs only when the user asks.
- Ignore `../deprecated/`.
