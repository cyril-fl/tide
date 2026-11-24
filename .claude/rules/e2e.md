---
paths:
  - "web/app/**"
  - "web/test/e2e/**"
---

# e2e that do not flake

- A new or changed test is done when it passes `bunx playwright test <spec> -g "<name>" --repeat-each 10`, not once. A CI failure is reproduced that way first (and `gh run list --workflow CI` tells if it recurs); no `retries`, they hide the real races (the ⌘Z one was a bug of the app).
- A test waits for what the app shows of the state its next step needs (the url, a toast, `waitForResponse`), not for a change that comes first: a list redrawn is not the work finished. Never a fixed timeout.
- In the app, what a later action reads (the undo stack, a created id) is written before the UI shows the change, or that action waits for it: not after `await refreshNuxtData()`, which the list beats.
