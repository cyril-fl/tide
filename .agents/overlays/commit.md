Notes (tide).

- Branch `develop`; the user pushes (`git push origin develop --follow-tags`). Other agents share the checkout: stage only your hunks.
- lefthook runs gofmt, Prettier and actionlint on staged files, and checks the prefix.
- Generated files (`docs/swagger.yaml`, `web/app/types/api.d.ts`): regenerate only when they hold nothing of another agent's, else stage your hunks.
- READMEs in English: `README.md` (CLI, API, server), `web/README.md` (the app).
