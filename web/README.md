# web

The web app of Tide, built with Nuxt and Nuxt UI, with no server of its own: the browser calls the API directly.

## Run

The whole stack, from the root of the repo: `make up`, then `make down` to stop it (Colima stays running, other projects may use it). `make up` starts the web app in the background, with its logs in `.build/web.log`.

Or piece by piece:

In `.env` at the root, allow the app:

```sh
WEB_ORIGIN=http://localhost:3000
```

```sh
make api                                 # at the root: PostgreSQL, Silo and the API on :8080
cd web && bun install && bun run dev    # http://localhost:3000
```

Do open `localhost`, not `127.0.0.1`: the API compares the origin exactly. `NUXT_PUBLIC_API_BASE` changes the address of the API.

The app asks you to log in. Without a session, it opens the login page, then goes back to the requested page. The account comes from an invitation link sent by an admin, from the server (`tide-server user add <name>`, see the [reference](../docs/reference.md#accounts)), or from the account creation page when the server opens it to everyone (`TIDE_ADD_USER_MODE=public`): the login page then shows a link to it, and hides it otherwise. On a database that is still empty, the login page leads straight to the creation of the first account, which becomes admin. The avatar at the bottom of the sidebar, collapsed or not, opens a menu: Account, Settings, Log out. The Account page changes the avatar (an image of the account, the initials without one), the name and the password. Changing the password logs out the other devices.

## Images

Pasting or dragging an image (PNG, JPEG, GIF, WebP, 20 MB at most) into a note sends it to the API and inserts it at that spot. A PDF, a zip, a sound or a video is inserted as a link with its name, which downloads it on click. It is stored in Silo, see the [reference](../docs/reference.md#images-and-attachments). When selected, an image offers its original size (the default one, with no title in the markdown) or three sizes: small (a third of the width), medium (two thirds) or large (the full width). The size is written in the markdown title, `![photo](/api/files/<id> "small")`, `"medium"` or `"large"`, like that of link cards: elsewhere, it stays a normal image.

## Shortcuts

`⌘K` or `/` opens the palette (notes, recent, actions, theme), `N` creates a note in the displayed folder, `T` a note that starts with a snippet, `⇧N` a folder, `⌘\` collapses the sidebar. Outside an input field only.

The sidebar is resized by dragging its edge (its width is kept from one visit to the next). Collapsed, it stays on screen as an icon bar (search, notes, graph, trash, settings, and the light/dark button except in system mode, where the OS decides). On a small screen, it becomes a drawer.

A right click on a folder makes it a favorite, a right click on a tag renames or deletes it.

The sidebar lists (Favorites, Pinned, Recent, Folders, Smart folders, Tags) are shown or hidden one by one in the settings, under the Navigation line of the Appearance section, stored on the account (`sidebar_sections`): the same on all devices. In advanced mode, the handle of each row reorders them by drag and drop; hiding the Folders requires Ultra mode.

When the note has changed on another device while you are writing it, the app merges the two texts line by line, from the text they had in common: changes in different places merge on their own (a toast says so); in the same place, the "Conflicts" dialog shows each passage with "Mine", "Theirs" or "Both", the text typed here being first kept in the history as a "conflict" version. If the merge fails (note locked elsewhere, network), "Reload" in the toast keeps the text typed here in the history, then reads the note again.

The source mode button, at the bottom of a note, shows its markdown as plain text in a CodeMirror editor, saved as you type like in the editor, which starts again from that text on return; the outline and the word count (advanced mode) are not shown there. `Ctrl+G` adds a cursor on the next occurrence of the selected word, `Ctrl+Shift+G` on all of them (macOS keys take `Ctrl+D` and `Cmd+F` before the editor).

A ` ```mermaid ` code block is displayed as a diagram (dark theme in dark mode), its code visible as long as the cursor is in it; a click on the diagram puts the cursor in it. LaTeX formulas `$…$` (inline) and `$$…$$` (block) are displayed with KaTeX, and a click opens their source to edit (on several lines for a block, `⌘↵` to confirm); `$5 and $10` stays text (no space right inside the dollars, no digit after). Diagrams and formulas are also displayed in an included note (`![[Note]]`). Mermaid and KaTeX are only loaded at the first note that contains one, and the markdown stays the one written. Comments, `%%…%%` as in Obsidian or `<!--…-->` as in HTML, inline or alone on their lines, are hidden in the editor as soon as typed (notes, snippets, included notes) and kept as written in the markdown, visible in the source mode.

The padlock at the bottom of a note locks it read-only (the editor and the source mode only read, the API refuses its text) until it is opened again.

In a folder, ⌘-click (Ctrl outside macOS) selects several items and ⇧-click a range: a bar offers to move or delete them, and dragging one of them takes the whole selection along. Esc cancels the selection.

The sort button of a folder's header orders it by name, date created, date modified or pinned items (pinned notes and favorite folders): a click on a choice sorts ascending (pinned first), a second descending, a third turns it off. The choices add up, the first one turned on deciding first (its rank shows beside it), and "Reset the sort" turns them all off: the neutral order, by name. Folders always come before notes. The choice is stored on the account (`sort`, `GET /api/items?sort=`): the same on all devices.

The graph moves by dragging and zooms with the wheel, by pinching the trackpad or with its + and − buttons. "Recenter" puts it back as at the start.

## Settings

The Settings page (the avatar menu at the bottom of the sidebar, or the palette) gathers what concerns the whole app. The settings are stored on the account (`PATCH /api/me`): they follow from one device to another. They are arranged in sections (General, Appearance, Editor, Storage, Data, Links in advanced mode, Advanced mode, and Users for an admin), each with its danger zone at the bottom when it has one (Data: erase everything), listed in `app/utils/settings.ts`. In advanced mode, the sidebar becomes the settings navigation (Account then a page per section, `/settings/<section>`), with a link back to the notes.

- **Language** of the interface, French or English.
- **Appearance**: the theme, chosen with a click on its thumbnail among ready-made color sets (Notes, Dracula, Catppuccin, Nord, Gruvbox, Everforest, Tokyo Night, Vue), and the mode (system, light, dark). A theme is declared in `app/utils/themes.ts`, its palettes in `main.css`, and its id in `domain.Themes` on the Go side. The mode button of the sidebar also saves, like that of the list view.
- **Notes with the same tags** (Appearance section): the number of notes sharing a tag shown around a note, in its linked notes and its graph: 20, 50 (default), 100 or all. The closest first, those sharing the most tags; the others are counted, with a link to the page of each tag.
- **After a merge** (Editor section): what becomes of a copy once merged. A copy is merged from its original note, like a merge request: the merge button of its toolbar lists its copies, and the dialog shows what the text becomes, with "Move the copy to the trash after the merge" and "Don't ask again". "Ask every time" (default) shows it at each merge, "Move the copy to the trash" and "Keep the copy" merge without it, conflicts still asked. "Undo the last merge" gives the note back its text, and the copy back from the trash, until the note is edited.
- **Users** (admin account only): the list of the server's accounts, with a search by name and 10 per page, and the deletion of an account — its notes, folders, tags and images go with it, after typing its name; neither your own, nor the last admin. The Invite button creates a single-use link, valid for 7 days, copied to the clipboard: the invited person chooses their own name and password. The pending invitations, folded under the list, each keep their link to copy (the server keeps the token encrypted with the admin's key) and can be revoked. The General section also shows the admin the server version (`GET /api/version`), to spot a deployment that is behind.
- **Plan** (Standard, Advanced or Ultra, the choice at the top of the Advanced mode section): advanced adds features, compared in the table under the choice; focus mode (`⌘.` or the button at the bottom of a note), which hides the sidebar until the page changes, and on a large screen a panel to the right of the note with its outline (a click on a heading scrolls to it), the graph of its neighbors and its linked notes, and the word count at the bottom of the note, updated as you type. The `/` menu arranges its commands in submenus (Headings, Lists, Callouts) and inserts a LaTeX formula or a Mermaid diagram, and the title of a callout follows the language of the account. Without advanced mode, the tables and callouts already written are displayed and kept when editing: only inserting them is reserved to advanced mode (an editor module without `feature` is loaded for everyone). A right click (or a long press) in a note opens the same menu. The `⌘K` palette adds "Move to…" for the displayed note or folder, "Rename" for the displayed note, folder or tag (a note takes the name in its first heading, added at the top if there is none) and "Change theme". The panel also shows the version history of the note (kept for everyone by the API, one per writing session, the session ending after 15 minutes of pause, and at least one per hour; the text of a version is read when it is chosen; on a phone, with no panel, the clock at the bottom of the note): a preview of each and "Restore this version", which replaces the text like a keystroke (saved, undoable with `⌘Z`). A "Compare with the current text" button shows there the lines of the version struck through and those of the current text underlined. The plans are listed in `plans` of `app/utils/features.ts`, each including the previous ones (a new one is added at the end, and to `domain.Plans` on the Go side); the level of each feature (the first plan that has it, or `off` for nobody) is declared in the same file: the app shows them with `usePro().has(…)`, and a page declared with `definePageMeta({ feature })` redirects to the home when it is not available. The Links section lists the broken `[[…]]` links (no note), ambiguous (several notes with the same title) or pointing to a deleted note, and the cited files the server does not have, with the note that contains them (`GET /api/links/broken`). The Storage section shows the space the images and attachments take on the server (`GET /api/files`), in a bar split by type (images, documents, sounds, videos) with its legend, like Apple's storage; advanced mode adds the list of files there, the heaviest first, 20 per page; in a note, an ambiguous link shows as dotted in the info color (a link with no note stays dashed gray), and a click on a pending link says which of the two cases, with the paths of the candidate notes. Advanced mode only touches the interface: the API answers the same.
- **Smart folders** (advanced mode): virtual folders, listed in the sidebar like favorites (section that can be hidden in the sidebar settings), created by the "Smart folder" button of the new folder dialog at the root and managed in their settings section. Each combines tags (any or all), included or excluded folders, creation and modification dates and a text, and is replayed at every opening (`GET /api/searches/{id}`). Its menu exports the notes it finds as a zip, side by side at its root (`GET /api/export?search=<id>`).
- **Snippets**: named markdown, whose `#tags` become the tags of the note where it lands. It is inserted at the cursor by typing `;;` in a note, then its name, or gives the starting text of a new note (`T`, or "New note from a snippet" in the palette); that note is only created once you type in it.
- **Trash**: the time before deleted items are erased for good (7, 30, 90, 365 days, or never).
- **Export** downloads all the notes as a zip of `.md` files, their images in `_files/`, like `tide export`.
- **Import** opens a dialog: the destination folder (the root by default), then a zip, `.md` files and their images and attachments (PDF, audio, video; dropped or chosen), or a whole folder (dropped or "Pick a folder", its tree kept). Like `tide import --parent`. A zip is imported alone. Two files with the same name outside a folder are reported and block the import: remove one. If a folder already exists at that spot, nothing is imported.

## Commands

```sh
bun run test        # save queue, links and graph layout
bun run e2e         # journeys in Chrome, against the running stack (make up); or make e2e from the root
                    # logs in with the e2e account (E2E_NAME, E2E_PASSWORD): tide-server user add e2e, after yours
                    # E2E_APP and E2E_API point the tests at another stack
bun run typecheck
bun run types       # regenerates app/types/api.d.ts from ../docs/swagger.yaml
bun run generate    # static site in .output/public
```
