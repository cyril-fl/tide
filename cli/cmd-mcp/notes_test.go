package cmdmcp

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNotes writes, reads, moves, trashes and restores through the API, by path as an agent does.
func TestNotes(t *testing.T) {
	agent := connect(t)

	var folder, inbox item
	call(t, agent, "create_item", map[string]any{"title": "Work"}, &folder)
	call(t, agent, "create_item", map[string]any{"title": "Inbox"}, &inbox)
	call(t, agent, "rename_folder", map[string]any{"folder": "/Work", "title": "Projects"}, &folder)
	if folder.Title != "Projects" {
		t.Errorf("rename_folder = %+v", folder)
	}

	var created note
	call(t, agent, "create_item", map[string]any{"content": "# Plan\nship it #go", "folder": "/Projects", "tags": []string{"mcp"}}, &created)
	if created.Path != "/Projects/Plan" || !slices.Equal(slices.Sorted(slices.Values(created.Tags)), []string{"go", "mcp"}) {
		t.Errorf("create_item = %+v", created)
	}

	// The #tags of the content stay, the tags given are added; the answer leaves the content out.
	var edited note
	call(t, agent, "edit_note", map[string]any{"note": "/Projects/Plan", "content": "# Plan\nshipped #go", "tags": []string{"done"}}, &edited)
	if edited.Content != "" || edited.Path != "/Projects/Plan" || !slices.Equal(slices.Sorted(slices.Values(edited.Tags)), []string{"done", "go"}) {
		t.Errorf("edit_note = %+v", edited)
	}
	var read batchNote
	read = readOne(t, agent, created.ID)
	if read.Content != "# Plan\nshipped #go\n\n#done" || read.CreatedAt == "" {
		t.Errorf("read_notes = %+v", read)
	}

	// A note without tags says so with [], and several notes are read in one call, in the order asked.
	var bare note
	call(t, agent, "create_item", map[string]any{"content": "# Bare"}, &bare)
	result := call(t, agent, "read_notes", map[string]any{"notes": []string{bare.ID}}, nil)
	if data, _ := json.Marshal(result.StructuredContent); !strings.Contains(string(data), `"tags":[]`) {
		t.Errorf("read_notes without tags = %s, want tags []", data)
	}
	var batched batch
	call(t, agent, "read_notes", map[string]any{"notes": []string{"/Bare", created.ID}}, &batched)
	both := batched.Notes
	if len(both) != 2 || both[0].ID != bare.ID || both[1].Content != "# Plan\nshipped #go\n\n#done" {
		t.Errorf("read_notes = %+v", both)
	}
	// A note never edited has no updated_at in the API: the agent gets created_at (#215).
	var datedBatch batch
	call(t, agent, "read_notes", map[string]any{"notes": []string{"/Bare"}}, &datedBatch)
	dated := datedBatch.Notes
	if len(dated) != 1 || dated[0].UpdatedAt == "" || dated[0].UpdatedAt != dated[0].CreatedAt {
		t.Errorf("read_notes = %+v, want updated_at = created_at", dated)
	}
	// max_chars gives the start of each content and its whole length; a short one is left whole.
	call(t, agent, "read_notes", map[string]any{"notes": []string{"/Bare", created.ID}, "max_chars": 7}, &batched)
	both = batched.Notes
	if len(both) != 2 || both[0].Truncated == nil || *both[0].Truncated || both[0].Content != "# Bare" || both[0].Length == nil || *both[0].Length != 6 ||
		both[1].Truncated == nil || !*both[1].Truncated || both[1].Content != "# Plan" || both[1].Length == nil || *both[1].Length != 25 {
		t.Errorf("read_notes max_chars = %+v", both)
	}
	// Without a text, the filters alone keep the untagged, the short notes and those with an empty section: Bare each
	// time, Plan neither.
	for _, args := range []map[string]any{{"untagged": true}, {"max_length": 10}, {"empty_sections": true}} {
		var short searchPage
		call(t, agent, "search_notes", args, &short)
		if len(short.Matches) != 1 || short.Matches[0].Id != bare.ID || short.Total != 1 {
			t.Errorf("search_notes %v = %+v, want Bare alone", args, short)
		}
	}
	// updated_before alone gives the notes changed before it (their order is that of the test transaction's now()).
	for date, want := range map[string]int{"2999-01-01": 2, "2000-01-01": 0} {
		var stale searchPage
		call(t, agent, "search_notes", map[string]any{"updated_before": date}, &stale)
		if len(stale.Matches) != want || stale.Total != want {
			t.Errorf("search_notes updated_before %s = %+v, want %d notes", date, stale, want)
		}
	}
	// The next page is empty, the total still counts Bare.
	var after searchPage
	call(t, agent, "search_notes", map[string]any{"untagged": true, "offset": 1}, &after)
	if len(after.Matches) != 0 || after.Total != 1 {
		t.Errorf("search_notes offset 1 = %+v, want no match of 1", after)
	}
	call(t, agent, "delete_item", map[string]any{"item": bare.ID}, nil)

	// tag_notes changes only the #tags, the rest of the text kept byte for byte, an unknown note reported.
	var tagRes tagResult
	call(t, agent, "tag_notes", map[string]any{"notes": []string{"/Projects/Plan", "/Nowhere"}, "add": []string{"old", "DONE"}, "remove": []string{"GO", "nope"}}, &tagRes)
	retagged := tagRes.Notes
	if len(retagged) != 2 || tagRes.Changed != 1 || tagRes.Unchanged != 0 || tagRes.Failed != 1 || retagged[0].Error != "" || retagged[0].ID != created.ID || retagged[0].Note != "" ||
		!slices.Equal(retagged[0].Added, []string{"old"}) || !slices.Equal(retagged[0].AlreadyPresent, []string{"DONE"}) ||
		!slices.Equal(retagged[0].Removed, []string{"GO"}) || !slices.Equal(retagged[0].NotPresent, []string{"nope"}) ||
		retagged[1].Error == "" || retagged[1].Note != "/Nowhere" {
		t.Errorf("tag_notes = %+v", tagRes)
	}
	read = readOne(t, agent, created.ID)
	if read.Content != "# Plan\nshipped\n\n#done #old" {
		t.Errorf("read_notes after tag_notes = %q", read.Content)
	}
	// #236: summary keeps the counts and only the failed notes.
	var brief tagResult
	call(t, agent, "tag_notes", map[string]any{"notes": []string{"/Projects/Plan", "/Nowhere"}, "add": []string{"old"}, "dry_run": true, "summary": true}, &brief)
	if brief.Unchanged != 1 || brief.Failed != 1 || len(brief.Notes) != 1 || brief.Notes[0].Note != "/Nowhere" || brief.Notes[0].Error == "" {
		t.Errorf("tag_notes summary = %+v", brief)
	}
	// tag_notes on what a search finds: dry_run lists the notes and changes nothing, then the call tags them.
	call(t, agent, "tag_notes", map[string]any{"text": "shipped", "add": []string{"review"}, "dry_run": true}, &tagRes)
	retagged = tagRes.Notes
	if len(retagged) != 1 || retagged[0].ID != created.ID || retagged[0].Title != "Plan" || !slices.Equal(retagged[0].Added, []string{"review"}) || !slices.Equal(retagged[0].Diff, []string{"- #done #old", "+ #done #old #review"}) {
		t.Errorf("tag_notes dry_run = %+v", retagged)
	}
	call(t, agent, "tag_notes", map[string]any{"text": "shipped", "add": []string{"review"}}, &tagRes)
	retagged = tagRes.Notes
	read = readOne(t, agent, created.ID)
	if len(retagged) != 1 || retagged[0].Error != "" || read.Content != "# Plan\nshipped\n\n#done #old #review" {
		t.Errorf("tag_notes by text = %+v, then %q", retagged, read.Content)
	}
	call(t, agent, "tag_notes", map[string]any{"text": "shipped", "remove": []string{"review"}}, nil)
	// #232: ops tag several groups in one call, counted apart; the second op sees the first.
	ops := []map[string]any{{"notes": []string{created.ID}, "add": []string{"a"}}, {"notes": []string{created.ID, "/Nowhere"}, "remove": []string{"a"}}}
	for _, dryRun := range []bool{true, false} {
		call(t, agent, "tag_notes", map[string]any{"ops": ops, "dry_run": dryRun}, &tagRes)
		read = readOne(t, agent, created.ID)
		if len(tagRes.Notes) != 3 || len(tagRes.Ops) != 2 || tagRes.Changed != 2 || tagRes.Failed != 1 || tagRes.Ops[0] != (tagCount{Changed: 1}) ||
			tagRes.Ops[1] != (tagCount{Changed: 1, Failed: 1}) || *tagRes.Notes[2].Op != 1 || read.Content != "# Plan\nshipped\n\n#done #old" {
			t.Errorf("tag_notes ops dry_run %v = %+v, then %q", dryRun, tagRes, read.Content)
		}
	}
	// A tag of an op is checked like a top-level one, before anything is sent.
	bad := []map[string]any{{"notes": []string{created.ID}, "add": []string{"a b"}}}
	if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "tag_notes", Arguments: map[string]any{"ops": bad}}); err != nil || !result.IsError ||
		!strings.Contains(result.Content[0].(*mcp.TextContent).Text, "a b") {
		t.Errorf("tag_notes ops with an invalid tag = %+v, %v", result, err)
	}
	// The filters of search_notes pick the notes too: tags_none and folder, dry_run first, then the call; "old" keeps Plan out.
	call(t, agent, "tag_notes", map[string]any{"tags_none": []string{"old"}, "folder": "/Projects", "add": []string{"x"}, "dry_run": true}, &tagRes)
	if len(tagRes.Notes) != 0 {
		t.Errorf("tag_notes tags_none old = %+v, want none", tagRes.Notes)
	}
	var side item
	call(t, agent, "create_item", map[string]any{"content": "# Side", "folder": "/Projects"}, &side)
	call(t, agent, "tag_notes", map[string]any{"tags_none": []string{"old"}, "folder": "/Projects", "add": []string{"x"}, "dry_run": true}, &tagRes)
	read = readOne(t, agent, side.ID)
	if len(tagRes.Notes) != 1 || tagRes.Notes[0].ID != side.ID || read.Content != "# Side" {
		t.Errorf("tag_notes tags_none dry_run = %+v, then %q", tagRes.Notes, read.Content)
	}
	call(t, agent, "tag_notes", map[string]any{"tags_none": []string{"old"}, "folder": "/Projects", "add": []string{"x"}}, &tagRes)
	read = readOne(t, agent, side.ID)
	if len(tagRes.Notes) != 1 || !strings.HasPrefix(read.Content, "# Side") || !strings.Contains(read.Content, "#x") {
		t.Errorf("tag_notes tags_none = %+v, then %q", tagRes.Notes, read.Content)
	}
	call(t, agent, "delete_item", map[string]any{"item": side.ID}, nil)
	// updated_before alone picks the notes too: none changed before 2000.
	call(t, agent, "tag_notes", map[string]any{"updated_before": "2000-01-01", "add": []string{"stale"}, "dry_run": true}, &tagRes)
	retagged = tagRes.Notes
	if len(retagged) != 0 {
		t.Errorf("tag_notes updated_before = %+v, want none", retagged)
	}

	var matches searchPage
	call(t, agent, "search_notes", map[string]any{"text": "shipped"}, &matches)
	var recent searchPage
	call(t, agent, "search_notes", map[string]any{"list": "recent"}, &recent)
	if len(matches.Matches) != 1 || len(recent.Recent) != 1 {
		t.Errorf("search_notes = %v, list recent = %v", matches, recent)
	}

	var moved changed
	var listed folderPage
	call(t, agent, "move_item", map[string]any{"item": "/Projects/Plan", "folder": "/Inbox"}, &moved)
	call(t, agent, "list_folder", map[string]any{"folder": "/Inbox"}, &listed)
	if len(moved.Items) != 1 || listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != created.ID || listed.Items[0].CreatedAt != "" {
		t.Errorf("move_item = %+v, list_folder = %+v", moved, listed)
	}
	// A locked note says so in the lists (#234), unlocked again for what follows.
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}
	lock := func(on bool) {
		if _, err := decode[apiclient.ReaditemResult](client.PatchApiNotesId(context.Background(), created.ID, apiclient.EditnoteBody{Readonly: &on})); err != nil {
			t.Fatal(err)
		}
	}
	lock(true)
	call(t, agent, "list_folder", map[string]any{"folder": "/Inbox"}, &listed)
	call(t, agent, "search_notes", map[string]any{"text": "shipped"}, &matches)
	if !listed.Items[0].Readonly || len(matches.Matches) != 1 || !apiclient.Deref(matches.Matches[0].Readonly) {
		t.Errorf("locked: list_folder = %+v, search_notes = %+v", listed, matches)
	}
	lock(false)
	// The dates only when asked.
	call(t, agent, "list_folder", map[string]any{"folder": "/Inbox", "dates": true}, &listed)
	if len(listed.Items) != 1 || listed.Items[0].CreatedAt == "" {
		t.Errorf("list_folder with dates = %+v", listed)
	}
	// The options, and the end marker: the one note of /Inbox, then a page of the root that goes on.
	call(t, agent, "list_folder", map[string]any{"folder": "/Inbox", "tags": true, "length": true, "preview": 3}, &listed)
	if it := listed.Items[0]; it.Tags == nil || it.Length == nil || *it.Length < 3 || len([]rune(it.Preview)) == 0 || len([]rune(it.Preview)) > 3 || listed.HasMore {
		t.Errorf("list_folder with tags, length and preview = %+v", listed)
	}
	call(t, agent, "list_folder", map[string]any{"limit": 1}, &listed)
	if len(listed.Items) != 1 || listed.Total != 2 || !listed.HasMore {
		t.Errorf("list_folder of the root, limit 1 = %+v", listed)
	}
	// A page past the end is empty (it leaves items out), the total still counts them all.
	listed = folderPage{}
	call(t, agent, "list_folder", map[string]any{"limit": 1, "offset": 5}, &listed)
	if listed.Total != 2 || len(listed.Items) != 0 {
		t.Errorf("list_folder of the root, offset 5 = %+v", listed)
	}

	// A folder goes to the trash with its content, and comes back with it.
	var removed, restored changed
	call(t, agent, "delete_item", map[string]any{"item": "/Inbox", "recursive": true}, &removed)
	if len(removed.Items) != 2 {
		t.Errorf("delete_item recursive = %+v", removed)
	}
	var trash folderPage
	call(t, agent, "list_folder", map[string]any{"trash": true}, &trash)
	call(t, agent, "restore_item", map[string]any{"item": inbox.ID}, &restored)
	if len(trash.Trash) != 3 || trash.Total != 3 || len(restored.Items) != 2 { // The folder, and the Bare and Side notes deleted earlier.
		t.Errorf("list_folder trash = %+v, restore_item = %+v", trash, restored)
	}
	call(t, agent, "delete_item", map[string]any{"item": "/Inbox/Plan"}, &removed)
	if len(removed.Items) != 1 || removed.Items[0].ID != created.ID {
		t.Errorf("delete_item = %+v", removed)
	}

	// A folder's path without recursive is refused, not its subtree trashed; a note's with it too.
	call(t, agent, "create_item", map[string]any{"content": "# Kept", "folder": "/Projects"}, &created)
	for args, want := range map[string]string{"/Projects": "give recursive", "/Projects/Kept": "leave out recursive"} {
		result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_item", Arguments: map[string]any{"item": args, "recursive": args == "/Projects/Kept"}})
		if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, want) {
			t.Errorf("delete_item %s = %+v, %v; want %q", args, result, err, want)
		}
	}
	// A list of the trash takes no folder.
	if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_folder", Arguments: map[string]any{"trash": true, "folder": "/Projects"}}); err != nil || !result.IsError {
		t.Errorf("list_folder trash with a folder = %+v, %v; want an error", result, err)
	}

	// A path of the other kind names the tool to use, not just "no item".
	for name, args := range map[string]map[string]any{
		"read_notes":  {"notes": []string{"/Projects"}},
		"list_folder": {"folder": "/Projects/Kept"},
	} {
		want := map[string]string{"read_notes": "use list_folder", "list_folder": "use read_notes"}[name]
		result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, want) {
			t.Errorf("%s of the other kind = %+v, %v, want %q", name, result, err, want)
		}
	}
}

// TestEditText renames a note and corrects a part of it without sending it whole (#224).
func TestEditText(t *testing.T) {
	agent := connect(t)
	fails := func(name string, args map[string]any, want string) {
		t.Helper()
		result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, want) {
			t.Errorf("%s %v = %+v, %v; want an error with %q", name, args, result, err, want)
		}
	}

	var created note
	var renamed partEdit
	call(t, agent, "create_item", map[string]any{"content": "# Pierre blanche de nettoyage maisonIngrédients\n\none two two"}, &created)
	call(t, agent, "edit_note", map[string]any{"note": created.ID, "title": "Pierre blanche"}, &renamed)
	if renamed.Title != "Pierre blanche" || renamed.Path != "/Pierre blanche" || renamed.Content != "" {
		t.Errorf("edit_note title = %+v", renamed)
	}

	var replaced partEdit
	call(t, agent, "edit_note", map[string]any{"note": "/Pierre blanche", "old": "one", "new": "un"}, &replaced)
	if replaced.Replaced != 1 || replaced.ID != created.ID {
		t.Errorf("edit_note old = %+v", replaced)
	}
	fails("edit_note", map[string]any{"note": created.ID, "old": "two", "new": "deux"}, "2 times")
	fails("edit_note", map[string]any{"note": created.ID, "old": "three"}, "not found")
	call(t, agent, "edit_note", map[string]any{"note": created.ID, "old": "two", "new": "deux", "all": true}, &replaced)
	read := readOne(t, agent, created.ID)
	if replaced.Replaced != 2 || read.Content != "# Pierre blanche\n\nun deux deux" {
		t.Errorf("edit_note old all = %+v, read = %q", replaced, read.Content)
	}
	// A section's body becomes new, the heading kept.
	call(t, agent, "edit_note", map[string]any{"note": created.ID, "content": "# Pierre blanche\n\n## Steps\nold\n## End"}, nil)
	call(t, agent, "edit_note", map[string]any{"note": created.ID, "section": "Steps", "new": "new"}, &replaced)
	if read = readOne(t, agent, created.ID); replaced.Replaced != 1 || !strings.Contains(read.Content, "## Steps\n\nnew") || strings.Contains(read.Content, "old") || !strings.Contains(read.Content, "## End") {
		t.Errorf("edit_note section = %+v, read = %q", replaced, read.Content)
	}
	// One mode at a time, tags only beside content.
	fails("edit_note", map[string]any{"note": created.ID}, "give one of")
	fails("edit_note", map[string]any{"note": created.ID, "content": "# X", "old": "x"}, "give one of")
	fails("edit_note", map[string]any{"note": created.ID, "title": "X", "old": "x"}, "give one of")
	fails("edit_note", map[string]any{"note": created.ID, "old": "un", "tags": []string{"x"}}, "tag_notes")
	// create_item makes a note, several or a folder, one at a time.
	fails("create_item", map[string]any{"content": "# X", "title": "X"}, "give one of")
	fails("create_item", map[string]any{"content": "# X", "notes": []string{"# Y"}}, "give one of")
	fails("create_item", map[string]any{"folder": "/"}, "give one of")

	// The text before the first change of the session is in the history.
	var versions history
	call(t, agent, "note_history", map[string]any{"note": created.ID}, &versions)
	if len(versions.Versions) != 1 {
		t.Errorf("note_history = %+v, want the text before the rename", versions)
	}
}

// TestCreateNotes makes several notes in one call, all in the folder with the tags given, answered as read_notes does.
func TestCreateNotes(t *testing.T) {
	agent := connect(t)
	var several batch
	call(t, agent, "create_item", map[string]any{"notes": []string{"# Soup\nleeks", "# Tart\napples #dessert"}, "tags": []string{"recipe"}}, &several)
	if several.Common.Folder != "/" || len(several.Notes) != 2 || several.Notes[0].Title != "Soup" ||
		!slices.Equal(slices.Sorted(slices.Values(several.Notes[1].Tags)), []string{"dessert", "recipe"}) {
		t.Errorf("create_item notes = %+v", several)
	}
}

func TestFactor(t *testing.T) {
	read := []note{
		{ID: "1", Title: "A", Path: "/F/A", CreatedAt: "d", UpdatedAt: "e1"},
		{ID: "2", Title: "B", Path: "/F/B2", CreatedAt: "d", UpdatedAt: "e2"},
	}
	got := factor(read)
	if got.Common != (batchCommon{Folder: "/F/", CreatedAt: "d"}) || got.Notes[0].Path != "" || got.Notes[0].Name != "" || got.Notes[1].Name != "B2" ||
		got.Notes[0].CreatedAt != "" || got.Notes[0].UpdatedAt != "e1" || got.Notes[0].ID != "1" {
		t.Errorf("factor = %+v", got)
	}
	read[1].Path = "/G/B"
	if got := factor(read); got.Common.Folder != "" || got.Notes[1].Path != "/G/B" {
		t.Errorf("factor of other folders = %+v", got)
	}
	if got := factor(read[:1]); got.Common != (batchCommon{}) || got.Notes[0].Path != "/F/A" {
		t.Errorf("factor of one note = %+v", got)
	}
}
