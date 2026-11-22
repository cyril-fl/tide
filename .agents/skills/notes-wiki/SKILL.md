---
name: notes-wiki
description: Bring the GitHub wiki of Notes (../notes.wiki) up to the latest tag, catching up every version since the last one it knows - a Versions.md section, the current version on Home.md, the pages the change touches (Web-app, Titles-and-tags, Theme, Data), screenshots of a new screen, then commit and push the wiki. Use only when the user asks for it, never on your own after a fix or a tag: when the user says "wiki", "mets à jour le wiki", "doc utilisateur".
context: fork
model: sonnet
---

# Update the wiki

The wiki is a separate repo, `../notes.wiki` (from `develop/`), in English. It is updated only when the user asks, since doing it after every tag cost too much time and tokens: it catches up all the tags since the last version it knows.

## Start from GitHub

```sh
cd ../notes.wiki
git status --short          # the user's own edits: keep them, never stage them
git fetch && git log --oneline -3 HEAD origin/master
```

Work on `origin/master`. If the local `master` is behind, `git pull --ff-only`; if it diverged, stop and ask. `.private/` is the user's, never commit it.

## What to write

Find what changed since the last version the wiki knows (`grep -m1 '^## v' Versions.md`): `git tag -l --format='%(contents)'` of each newer tag, and `git log --format='%h %s' <last>..HEAD` in `develop/` (skip `test:`, `chore:`, `style:` unless the user sees it).

- **`Versions.md`**: one section per tag, newest first, above the previous one: `## vX.Y.Z: <short title> (<d> <abbreviated month> <year>)`, then bullets saying what the user gets. A manual step (an `ALTER`, an env var) goes in its bullet.
- **`Home.md`**: `Current version: **vX.Y.Z**.`
- **The page of the feature**, so that it describes the app as it is now, not as a changelog:
  - `Web-app.md`: screens, shortcuts, settings (in their section), Pro mode.
  - `Titles-and-tags.md`: titles, tags, `[[links]]`.
  - `Theme.md`: themes, modes, fonts.
  - `Data.md`: a new column or table in the mermaid diagram and the table list, encryption.
  A new page also goes into `_Sidebar.md` and the page list of `Home.md`.
- Never: CLI commands, API routes or installation (they live in the READMEs), Tailscale addresses or the user's network setup.

Check each claim against the code or the i18n strings (`web/i18n/locales/en.json`) for the exact labels; the tag message alone is too short.

## Follow the diff into every page

The source of truth is the diff since the last tag the wiki knows, not the wiki: `git diff --stat <last>..<new tag>` in `develop/`, then the diff of what the user sees. A change rarely stays on one page, so for each change grep every page but `Versions.md` (a history) for what it touches and fix each sentence it makes false, not only the feature's page:

- a renamed thing (a binary, an env var, a path, the repo, a label in `en.json`, a shortcut in `useShortcuts.ts`): grep its old name;
- a list that grew or shrank (`domain.Themes`, `domain.Features` and what each plan unlocks, the settings sections, the MCP tools of `MCP.md`, the tables and columns of `core/domain/`): every page that lists it;
- a fixed limit (an issue closed by a commit of the range): grep its number, and the sentence that described the limit;
- a screen that changed: its screenshot is retaken.

Say in the report which changes of the diff moved which pages, and what was left unchecked.

## Screenshots

A new screen or a visible change gets a PNG in `images/` (`app-<topic>.png`), linked from its page with an alt text in English. Use the `notes-app-screenshot` skill. For a shot that needs content or a click:

- write a throwaway Playwright script in the session scratchpad, run from `web/` with `NODE_PATH=$PWD/node_modules` (CommonJS `require`);
- create demo notes in a folder of the e2e account (`POST /api/folders`, `POST /api/notes` with `parent_id` and `content`), set the settings the shot needs (`PATCH /api/me`, e.g. `pro`), then in `finally` delete the folder (`DELETE /api/folders/{id}`, then `DELETE /api/trash/{id}`) and put back only the settings changed;
- hide the Nuxt devtools button before the shot: `page.addStyleTag({ content: '#nuxt-devtools-container{display:none!important}' })`;
- look at every PNG before copying it to `images/`.

## Commit and push

```sh
git add <pages> <images>    # explicit paths
git commit -m "docs: vX.Y.Z, <what the pages now cover>"
git push origin master
```

The agent pushes the wiki, unlike `develop`: an unpushed wiki commit is what left the online wiki behind. Say in the report that it was pushed.
