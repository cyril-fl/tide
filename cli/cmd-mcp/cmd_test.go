package cmdmcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/exportnotes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect serves the tools on the API of apitest to an MCP client in memory.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	apitest.Serve(t)
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "notes"}, nil)
	AddTools(server, client, false)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { agent.Close() })
	return agent
}

// call runs a tool and decodes its structured answer into out, failing the test on an error.
func call(t *testing.T, agent *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: %v", name, result.Content[0].(*mcp.TextContent).Text)
	}
	if out != nil {
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s: %v: %s", name, err, data)
		}
	}
	return result
}

// TestAddTools catches a schema the SDK cannot infer from the arguments: the tools would be
// announced broken, and only an agent calling them would find out.
func TestAddTools(t *testing.T) {
	agent := connect(t)
	result, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil || tool.Description == "" {
			t.Errorf("tool %q: schema %v, description %q", tool.Name, tool.InputSchema, tool.Description)
		}
		// The client asks before a destructive tool and runs a read-only one freely: a tool without hints is
		// destructive by default, and one marked wrong deletes without asking.
		a := tool.Annotations
		switch {
		case a == nil:
			t.Errorf("tool %q: no annotations", tool.Name)
		case strings.HasPrefix(tool.Name, "delete_") || tool.Name == "edit_note" || tool.Name == "replace_notes" || tool.Name == "save_snippet" || tool.Name == "save_search":
			if a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
				t.Errorf("tool %q: %+v, want destructive", tool.Name, a)
			}
		case slices.Contains([]string{"search_notes", "read_note", "read_notes", "list_folder", "export_notes", "read_file", "snippets", "saved_searches"}, tool.Name):
			if !a.ReadOnlyHint {
				t.Errorf("tool %q: %+v, want read-only", tool.Name, a)
			}
		}
	}
	for _, want := range []string{"search_notes", "recent_notes", "duplicate_notes", "read_note", "read_notes", "list_folder", "create_note", "edit_note", "create_folder", "rename_folder", "move_item", "delete_note", "delete_folder", "list_trash", "restore_item", "tags", "tag_notes", "rename_tag", "replace_notes", "delete_tag", "note_history", "note_links", "read_file", "attach_file", "export_notes", "import_notes", "snippets", "save_snippet", "delete_snippet", "saved_searches", "save_search", "delete_search"} {
		if !slices.Contains(names, want) {
			t.Errorf("tools = %v, missing %q", names, want)
		}
	}
}

// TestInstructions catches instructions naming a tool or an argument the server does not announce, renamed or only
// in the writing tools: the agent would look for it in vain.
func TestInstructions(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		ctx := context.Background()
		client, err := apiclient.NewClientWithResponses("http://unused") // listing the tools calls no route
		if err != nil {
			t.Fatal(err)
		}
		server := mcp.NewServer(&mcp.Implementation{Name: "notes"}, &mcp.ServerOptions{Instructions: Instructions(readOnly)})
		AddTools(server, client, readOnly)
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		session, err := server.Connect(ctx, serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err := agent.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		known := map[string]bool{}
		for _, tool := range result.Tools {
			known[tool.Name] = true
			properties, _ := tool.InputSchema.(map[string]any)["properties"].(map[string]any) // none without arguments
			for name := range properties {
				known[name] = true
			}
		}
		instructions := agent.InitializeResult().Instructions
		for _, name := range regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`).FindAllString(instructions, -1) {
			if !known[name] {
				t.Errorf("readOnly %v: the instructions name %q, which no tool announces", readOnly, name)
			}
		}
		agent.Close()
		session.Close()
	}
}

// TestNotes writes, reads, moves, trashes and restores through the API, by path as an agent does.
func TestNotes(t *testing.T) {
	agent := connect(t)

	var folder, inbox item
	call(t, agent, "create_folder", map[string]any{"title": "Work"}, &folder)
	call(t, agent, "create_folder", map[string]any{"title": "Inbox"}, &inbox)
	call(t, agent, "rename_folder", map[string]any{"folder": "/Work", "title": "Projects"}, &folder)
	if folder.Title != "Projects" {
		t.Errorf("rename_folder = %+v", folder)
	}

	var created note
	call(t, agent, "create_note", map[string]any{"content": "# Plan\nship it #go", "folder": "/Projects", "tags": []string{"mcp"}}, &created)
	if created.Path != "/Projects/Plan" || !slices.Equal(slices.Sorted(slices.Values(created.Tags)), []string{"go", "mcp"}) {
		t.Errorf("create_note = %+v", created)
	}

	// The #tags of the content stay, the tags given are added; the answer leaves the content out.
	var edited note
	call(t, agent, "edit_note", map[string]any{"note": "/Projects/Plan", "content": "# Plan\nshipped #go", "tags": []string{"done"}}, &edited)
	if edited.Content != "" || edited.Path != "/Projects/Plan" || !slices.Equal(slices.Sorted(slices.Values(edited.Tags)), []string{"done", "go"}) {
		t.Errorf("edit_note = %+v", edited)
	}
	var read note
	call(t, agent, "read_note", map[string]any{"note": created.ID}, &read)
	if read.Content != "# Plan\nshipped #go\n\n#done" || read.CreatedAt == "" {
		t.Errorf("read_note = %+v", read)
	}

	// A note without tags says so with [], and several notes are read in one call, in the order asked.
	var bare note
	call(t, agent, "create_note", map[string]any{"content": "# Bare"}, &bare)
	result := call(t, agent, "read_note", map[string]any{"note": bare.ID}, nil)
	if data, _ := json.Marshal(result.StructuredContent); !strings.Contains(string(data), `"tags":[]`) {
		t.Errorf("read_note without tags = %s, want tags []", data)
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
	// Without a text, the filters alone keep the untagged and the short notes: Bare each time, Plan neither.
	for _, args := range []map[string]any{{"untagged": true}, {"max_length": 10}} {
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
	call(t, agent, "delete_note", map[string]any{"note": bare.ID}, nil)

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
	call(t, agent, "read_note", map[string]any{"note": created.ID}, &read)
	if read.Content != "# Plan\nshipped\n\n#done #old" {
		t.Errorf("read_note after tag_notes = %q", read.Content)
	}
	// tag_notes on what a search finds: dry_run lists the notes and changes nothing, then the call tags them.
	call(t, agent, "tag_notes", map[string]any{"text": "shipped", "add": []string{"review"}, "dry_run": true}, &tagRes)
	retagged = tagRes.Notes
	if len(retagged) != 1 || retagged[0].ID != created.ID || retagged[0].Title != "Plan" || !slices.Equal(retagged[0].Added, []string{"review"}) || !slices.Equal(retagged[0].Diff, []string{"- #done #old", "+ #done #old #review"}) {
		t.Errorf("tag_notes dry_run = %+v", retagged)
	}
	call(t, agent, "tag_notes", map[string]any{"text": "shipped", "add": []string{"review"}}, &tagRes)
	retagged = tagRes.Notes
	call(t, agent, "read_note", map[string]any{"note": created.ID}, &read)
	if len(retagged) != 1 || retagged[0].Error != "" || read.Content != "# Plan\nshipped\n\n#done #old #review" {
		t.Errorf("tag_notes by text = %+v, then %q", retagged, read.Content)
	}
	call(t, agent, "tag_notes", map[string]any{"text": "shipped", "remove": []string{"review"}}, nil)
	// updated_before alone picks the notes too: none changed before 2000.
	call(t, agent, "tag_notes", map[string]any{"updated_before": "2000-01-01", "add": []string{"stale"}, "dry_run": true}, &tagRes)
	retagged = tagRes.Notes
	if len(retagged) != 0 {
		t.Errorf("tag_notes updated_before = %+v, want none", retagged)
	}

	var matches searchPage
	call(t, agent, "search_notes", map[string]any{"text": "shipped"}, &matches)
	var recent []map[string]any
	call(t, agent, "recent_notes", map[string]any{}, &recent)
	if len(matches.Matches) != 1 || len(recent) != 1 {
		t.Errorf("search_notes = %v, recent_notes = %v", matches, recent)
	}

	var moved []item
	var listed folderPage
	call(t, agent, "move_item", map[string]any{"item": "/Projects/Plan", "folder": "/Inbox"}, &moved)
	call(t, agent, "list_folder", map[string]any{"folder": "/Inbox"}, &listed)
	if len(moved) != 1 || listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != created.ID || listed.Items[0].CreatedAt != "" {
		t.Errorf("move_item = %+v, list_folder = %+v", moved, listed)
	}
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
	// A page past the end is empty, the total still counts them all.
	call(t, agent, "list_folder", map[string]any{"limit": 1, "offset": 5}, &listed)
	if listed.Total != 2 || len(listed.Items) != 0 {
		t.Errorf("list_folder of the root, offset 5 = %+v", listed)
	}

	// A folder goes to the trash with its content, and comes back with it.
	var removed, restored []item
	call(t, agent, "delete_folder", map[string]any{"folder": "/Inbox"}, &removed)
	if len(removed) != 2 {
		t.Errorf("delete_folder = %+v", removed)
	}
	var trash []map[string]any
	call(t, agent, "list_trash", map[string]any{}, &trash)
	call(t, agent, "restore_item", map[string]any{"item": inbox.ID}, &restored)
	if len(trash) != 2 || len(restored) != 2 { // The folder and the Bare note deleted earlier.
		t.Errorf("list_trash = %v, restore_item = %+v", trash, restored)
	}
	call(t, agent, "delete_note", map[string]any{"note": "/Inbox/Plan"}, &removed)
	if len(removed) != 1 || removed[0].ID != created.ID {
		t.Errorf("delete_note = %+v", removed)
	}

	// A folder's path given to delete_note is refused, not its subtree trashed.
	result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_note", Arguments: map[string]any{"note": "/Projects"}})
	if err != nil || !result.IsError {
		t.Errorf("delete_note of a folder = %+v, %v", result, err)
	}

	// A path of the other kind names the tool to use, not just "no item".
	call(t, agent, "create_note", map[string]any{"content": "# Kept", "folder": "/Projects"}, &created)
	for name, args := range map[string]map[string]any{
		"read_note":   {"note": "/Projects"},
		"read_notes":  {"notes": []string{"/Projects"}},
		"list_folder": {"folder": "/Projects/Kept"},
	} {
		want := map[string]string{"read_note": "use list_folder", "read_notes": "use list_folder", "list_folder": "use read_note"}[name]
		result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, want) {
			t.Errorf("%s of the other kind = %+v, %v, want %q", name, result, err, want)
		}
	}
}

// TestTagsHistoryLinks reads what surrounds a note: its tags, its versions, its links.
func TestTagsHistoryLinks(t *testing.T) {
	agent := connect(t)

	var target, source note
	call(t, agent, "create_note", map[string]any{"content": "# Target\n#go"}, &target)
	call(t, agent, "create_note", map[string]any{"content": "# Source\nsee [[Target]] and [[Nowhere]] #go"}, &source)

	call(t, agent, "create_note", map[string]any{"content": "# Other\n#golf #rust"}, nil)
	var counted, tagged, dated tags
	call(t, agent, "tags", map[string]any{"prefix": "GO", "min_notes": 2}, &counted)
	call(t, agent, "tags", map[string]any{"tag": "go", "limit": 1}, &tagged)
	call(t, agent, "tags", map[string]any{"tag": "go", "limit": 1, "dates": true}, &dated)
	if len(counted.Tags) != 1 || counted.Tags[0].Value != "go" || counted.Tags[0].Notes != 2 || tagged.Tag != "go" || len(tagged.Notes) != 1 || tagged.Total != 2 || tagged.Notes[0].CreatedAt != "" || len(dated.Notes) != 1 || dated.Notes[0].CreatedAt == "" {
		t.Errorf("tags = %+v, %+v, %+v", counted, tagged, dated)
	}
	var all tags
	call(t, agent, "tags", map[string]any{"prefix": "#go"}, &all)
	if len(all.Tags) != 2 {
		t.Errorf("tags prefix #go = %+v, want go and golf", all)
	}

	var linked, broken links
	call(t, agent, "note_links", map[string]any{"note": "/Target"}, &linked)
	call(t, agent, "note_links", map[string]any{}, &broken)
	if len(linked.Backlinks) != 1 || len(linked.Neighbors) != 1 || len(broken.Broken) != 1 {
		t.Errorf("note_links = %+v, broken = %+v", linked, broken)
	}

	// The first save of a session keeps the text it replaces.
	call(t, agent, "edit_note", map[string]any{"note": "/Target", "content": "# Target\nnew #go"}, nil)
	var versions, version history
	call(t, agent, "note_history", map[string]any{"note": "/Target"}, &versions)
	if len(versions.Versions) != 1 {
		t.Fatalf("note_history = %+v", versions)
	}
	call(t, agent, "note_history", map[string]any{"note": "/Target", "version": versions.Versions[0].Id}, &version)
	if version.Version == nil || apiclient.Deref(version.Version.Content) != target.Content {
		t.Errorf("note_history version = %+v", version)
	}

	var renamed struct {
		Value string `json:"value"`
	}
	// dry_run previews the notes rewritten and writes nothing: the real call then renames the tag.
	var preview struct {
		Value string `json:"value"`
		Notes []struct {
			Path string   `json:"path"`
			Diff []string `json:"diff"`
		} `json:"notes"`
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "go", "to": "golang", "dry_run": true}, &preview)
	if preview.Value != "golang" || len(preview.Notes) == 0 || len(preview.Notes[0].Diff) == 0 {
		t.Errorf("rename_tag dry_run = %+v", preview)
	}
	var still tags
	call(t, agent, "tags", map[string]any{"tag": "go", "limit": 1}, &still)
	if still.Total == 0 {
		t.Errorf("rename_tag dry_run renamed the tag: %+v", still)
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "go", "to": "golang"}, &renamed)
	// A name taken refuses, unless merged.
	call(t, agent, "create_note", map[string]any{"content": "# Merged\n#gl"}, nil)
	if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "rename_tag", Arguments: map[string]any{"tag": "gl", "to": "golang"}}); err != nil || !result.IsError {
		t.Errorf("rename_tag onto a taken name = %+v, %v; want an error", result, err)
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "gl", "to": "golang", "merge": true}, &renamed)
	var replaced struct {
		Notes       int `json:"notes"`
		Occurrences int `json:"occurrences"`
	}
	call(t, agent, "replace_notes", map[string]any{"find": "golang", "replace": "go", "dry_run": true}, &replaced)
	if replaced.Notes == 0 {
		t.Errorf("replace_notes dry_run = %+v, want the notes holding golang", replaced)
	}
	var deleted []map[string]any
	call(t, agent, "delete_tag", map[string]any{"tag": "golang"}, &deleted)
	if renamed.Value != "golang" || len(deleted) != 1 {
		t.Errorf("rename_tag = %+v, delete_tag = %v", renamed, deleted)
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

	var created, renamed note
	call(t, agent, "create_note", map[string]any{"content": "# Pierre blanche de nettoyage maisonIngrédients\n\none two two"}, &created)
	call(t, agent, "rename_note", map[string]any{"note": created.ID, "title": "Pierre blanche"}, &renamed)
	if renamed.Title != "Pierre blanche" || renamed.Path != "/Pierre blanche" || renamed.Content != "" {
		t.Errorf("rename_note = %+v", renamed)
	}

	var replaced partEdit
	call(t, agent, "replace_in_note", map[string]any{"note": "/Pierre blanche", "old": "one", "new": "un"}, &replaced)
	if replaced.Replaced != 1 || replaced.ID != created.ID {
		t.Errorf("replace_in_note = %+v", replaced)
	}
	fails("replace_in_note", map[string]any{"note": created.ID, "old": "two", "new": "deux"}, "2 times")
	fails("replace_in_note", map[string]any{"note": created.ID, "old": "three"}, "not found")
	call(t, agent, "replace_in_note", map[string]any{"note": created.ID, "old": "two", "new": "deux", "all": true}, &replaced)
	var read note
	call(t, agent, "read_note", map[string]any{"note": created.ID}, &read)
	if replaced.Replaced != 2 || read.Content != "# Pierre blanche\n\nun deux deux" {
		t.Errorf("replace_in_note all = %+v, read = %q", replaced, read.Content)
	}

	// The text before the first change of the session is in the history.
	var versions history
	call(t, agent, "note_history", map[string]any{"note": created.ID}, &versions)
	if len(versions.Versions) != 1 {
		t.Errorf("note_history = %+v, want the text before the rename", versions)
	}
}

// TestFiles attaches an image, reads it back, then exports and imports notes.
func TestFiles(t *testing.T) {
	agent := connect(t)

	// The smallest GIF, which the server recognizes from its bytes.
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	var file attached
	call(t, agent, "attach_file", map[string]any{"name": "dot.gif", "data": base64.StdEncoding.EncodeToString(gif)}, &file)
	if file.Markdown != "![dot.gif](/api/files/"+file.ID+")" {
		t.Errorf("attach_file = %+v", file)
	}
	result := call(t, agent, "read_file", map[string]any{"file": file.ID}, nil)
	if image, ok := result.Content[0].(*mcp.ImageContent); !ok || !bytes.Equal(image.Data, gif) || image.MIMEType != "image/gif" {
		t.Errorf("read_file = %+v", result.Content[0])
	}
	var files []map[string]any
	call(t, agent, "read_file", map[string]any{}, &files)
	if len(files) != 1 {
		t.Errorf("read_file list = %v", files)
	}

	var imported struct{ Folders, Notes int }
	call(t, agent, "import_notes", map[string]any{"files": []importFile{{Path: "Trip/Day.md", Content: "# Day\n" + file.Markdown}}}, &imported)
	if imported.Folders != 1 || imported.Notes != 1 {
		t.Errorf("import_notes = %+v", imported)
	}
	var out export
	call(t, agent, "export_notes", map[string]any{"folder": "/Trip"}, &out)
	if out.Total != 2 || len(out.Files) != 2 || out.Files[0].Path != "Day.md" || !strings.Contains(out.Files[0].Content, "# Day") || out.Files[1].File != file.ID {
		t.Errorf("export_notes = %+v", out)
	}
	// The path is not enough to read a note back: its id is.
	var day note
	call(t, agent, "read_note", map[string]any{"note": out.Files[0].ID}, &day)
	if day.Path != "/Trip/Day" || out.Files[0].UpdatedAt == "" {
		t.Errorf("export_notes id = %+v, read_note = %+v", out.Files[0], day)
	}
	call(t, agent, "export_notes", map[string]any{"folder": "/Trip", "offset": 1}, &out)
	if out.Total != 2 || len(out.Files) != 1 || out.Files[0].File != file.ID {
		t.Errorf("export_notes, offset 1 = %+v", out)
	}

	// A path climbing out of the import is refused before anything is sent.
	bad, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "import_notes", Arguments: map[string]any{"files": []importFile{{Path: "../x.md", Content: "x"}}}})
	if err != nil || !bad.IsError {
		t.Errorf("import_notes ../x.md = %+v, %v", bad, err)
	}
}

// TestSnippets creates, replaces and removes snippets one at a time, the others kept, the server's checks surfacing.
func TestSnippets(t *testing.T) {
	agent := connect(t)

	var snippets []apiclient.DomainSnippet
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if len(snippets) != 0 {
		t.Errorf("snippets of a new account = %+v", snippets)
	}
	call(t, agent, "save_snippet", map[string]any{"name": "Book", "content": "# Title\n#reading"}, nil)
	call(t, agent, "save_snippet", map[string]any{"name": "Trip", "content": "# Trip"}, nil)
	var saved apiclient.DomainSnippet
	call(t, agent, "save_snippet", map[string]any{"name": "Book", "content": "# Book"}, &saved)
	want := []apiclient.DomainSnippet{{Name: "Book", Content: "# Book"}, {Name: "Trip", Content: "# Trip"}}
	if saved != want[0] {
		t.Errorf("save_snippet = %+v, want only %+v", saved, want[0])
	}
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want) {
		t.Errorf("snippets = %+v, want %+v", snippets, want)
	}

	call(t, agent, "delete_snippet", map[string]any{"name": "Book"}, nil)
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want[1:]) {
		t.Errorf("snippets after delete_snippet = %+v, want %+v", snippets, want[1:])
	}

	// An unknown name and a nameless snippet (refused by the API) are errors, the list left as it was.
	for name, args := range map[string]map[string]any{"delete_snippet": {"name": "Book"}, "save_snippet": {"name": " ", "content": "x"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args}); err != nil || !result.IsError {
			t.Errorf("%s %v = %+v, %v; want an error", name, args, result, err)
		}
	}
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want[1:]) {
		t.Errorf("snippets after the errors = %+v", snippets)
	}
}

// TestSavedSearches saves searches by tag name and folder path, runs them a page at a time, replaces and removes one,
// then refuses all of it once the account leaves the Pro plan.
func TestSavedSearches(t *testing.T) {
	agent := connect(t)
	call(t, agent, "create_folder", map[string]any{"title": "Work"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Pasta\n#cuisine #italie", "folder": "/Work"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Soup\n#cuisine"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Rome\n#italie"}, nil)

	var saved savedSearch
	call(t, agent, "save_search", map[string]any{"name": "Italian", "tags": []string{"cuisine", "#Italie"}, "tag_mode": "all"}, &saved)
	if saved.ID == "" || !slices.Equal(saved.Tags, []string{"cuisine", "italie"}) || saved.TagMode != "all" {
		t.Errorf("save_search = %+v", saved)
	}
	var run searches
	call(t, agent, "saved_searches", map[string]any{"search": "italian"}, &run)
	if run.Total != 1 || len(run.Notes) != 1 || run.Notes[0].Path != "/Work/Pasta" || run.Search == nil || run.Search.ID != saved.ID {
		t.Errorf("saved_searches italian = %+v", run)
	}

	call(t, agent, "save_search", map[string]any{"name": "Elsewhere", "folders": []string{"/Work"}, "folder_mode": "exclude", "created_to": "2999-12-31"}, &saved)
	if !slices.Equal(saved.Folders, []string{"/Work"}) || saved.CreatedTo != "2999-12-31T23:59:59.999Z" {
		t.Errorf("save_search Elsewhere = %+v", saved)
	}
	call(t, agent, "saved_searches", map[string]any{"search": saved.ID}, &run)
	if run.Total != 2 {
		t.Errorf("saved_searches Elsewhere = %+v", run)
	}

	// Replacing keeps the id and drops the criteria left out; a page still counts every note.
	call(t, agent, "save_search", map[string]any{"search": "Elsewhere", "name": "Cooking", "tags": []string{"cuisine"}}, nil)
	var replaced searches // a fresh one: decoding into run would keep the fields the answer leaves out
	call(t, agent, "saved_searches", map[string]any{"search": saved.ID, "limit": 1}, &replaced)
	if replaced.Total != 2 || len(replaced.Notes) != 1 || replaced.Search.Name != "Cooking" || len(replaced.Search.Folders) != 0 || replaced.Search.TagMode != "" {
		t.Errorf("saved_searches Cooking = %+v, %+v", replaced, replaced.Search)
	}

	var left []savedSearch
	call(t, agent, "delete_search", map[string]any{"search": "Italian"}, &left)
	var listed searches
	call(t, agent, "saved_searches", map[string]any{}, &listed)
	if len(left) != 1 || len(listed.Searches) != 1 || listed.Searches[0].ID != saved.ID {
		t.Errorf("delete_search = %+v, saved_searches = %+v", left, listed)
	}

	// An unknown tag, folder, search or a date that is none are errors.
	for name, args := range map[string]map[string]any{
		"save_search":    {"name": "x", "tags": []string{"nope"}},
		"saved_searches": {"search": "Italian"},
		"delete_search":  {"search": "Italian"},
	} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args}); err != nil || !result.IsError {
			t.Errorf("%s %v = %+v, %v; want an error", name, args, result, err)
		}
	}
	for _, args := range []map[string]any{{"name": "x", "folders": []string{"/Nowhere"}}, {"name": "x", "edited_from": "yesterday"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_search", Arguments: args}); err != nil || !result.IsError {
			t.Errorf("save_search %v = %+v, %v; want an error", args, result, err)
		}
	}

	// Outside the Pro plan, every saved search tool says so.
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}
	base := domain.Plans[0]
	if _, err := decode[apiclient.DomainUser](client.PatchApiMe(context.Background(), apiclient.EdituserInput{Plan: &base})); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string]map[string]any{"saved_searches": {}, "save_search": {"name": "x"}, "delete_search": {"search": "Cooking"}} {
		result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "Pro plan") {
			t.Errorf("%s outside Pro = %+v, %v; want the Pro plan error", name, result, err)
		}
	}
}

// TestImportDir refuses what would climb out of the import or is no note.
func TestImportDir(t *testing.T) {
	dir, err := importDir([]importFile{{Path: "Folder/Note.md", Content: "# Note"}})
	if err != nil || string(dir["Folder/Note.md"].Data) != "# Note" {
		t.Fatalf("importDir = %v, %v", dir, err)
	}
	for _, bad := range []string{"../Note.md", "/Note.md", "Folder/../Note.md", "image.png", ""} {
		if _, err := importDir([]importFile{{Path: bad}}); !errors.Is(err, errBadPath) {
			t.Errorf("importDir(%q) = %v, want errBadPath", bad, err)
		}
	}
}

// TestExportedFiles drops the directories, keeps a note's text and names a file by its id; a note of a folder
// named like filesDir stays a note.
func TestExportedFiles(t *testing.T) {
	id := "0b6f8e2a-1c3d-4e5f-8a9b-0c1d2e3f4a5b"
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	modified := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	for name, content := range map[string]string{"Folder/": "", "Folder/Note.md": "text", filesDir + "/" + id + ".png": "bytes", filesDir + "/Other.md": "note"} {
		header := &zip.FileHeader{Name: name}
		if name == "Folder/Note.md" {
			header.Comment, header.Modified = "note-id", modified
		}
		entry, _ := archive.CreateHeader(header)
		entry.Write([]byte(content))
	}
	archive.Close()
	reader, err := zip.NewReader(bytes.NewReader(body.Bytes()), int64(body.Len()))
	if err != nil {
		t.Fatal(err)
	}

	got, err := exportedFiles(reader)
	slices.SortFunc(got, func(a, b exported) int { return strings.Compare(a.Path, b.Path) })
	want := []exported{{Path: "Folder/Note.md", ID: "note-id", UpdatedAt: "2026-11-02T09:30:00Z", Content: "text"}, {Path: filesDir + "/" + id + ".png", File: id}, {Path: filesDir + "/Other.md", Content: "note"}}
	slices.SortFunc(want, func(a, b exported) int { return strings.Compare(a.Path, b.Path) })
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("exportedFiles = %v, %v, want %v", got, err, want)
	}
}

func TestPage(t *testing.T) {
	items := []int{0, 1, 2, 3, 4}
	tests := map[string]struct {
		limit, offset int
		want          []int
	}{
		"every item":   {0, 0, items},
		"first page":   {2, 0, []int{0, 1}},
		"last page":    {2, 4, []int{4}},
		"past the end": {2, 9, []int{}},
		"all after":    {0, 3, []int{3, 4}},
	}
	for name, test := range tests {
		if got := page(items, test.limit, test.offset); !slices.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", name, got, test.want)
		}
	}
}

// The CLI has no kernel to import them from: they must stay those of the server.
func TestServerNames(t *testing.T) {
	if filesDir != exportnotes.FilesDir || fileURL("id") != domain.FileURL("id") {
		t.Errorf("filesDir %q, fileURL %q; want %q, %q", filesDir, fileURL("id"), exportnotes.FilesDir, domain.FileURL("id"))
	}
}

// TestEditMeRetries reads the account again and edits over it when another write lands between its read and its save.
func TestEditMeRetries(t *testing.T) {
	ctx := context.Background()
	apitest.Serve(t)
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	runs := 0
	me, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
		runs++
		if runs == 1 {
			// The app saves a snippet after the read: the first save must be refused, not overwrite it.
			app := []apiclient.DomainSnippet{{Name: "App"}}
			if _, err := decode[apiclient.DomainUser](client.PatchApiMe(ctx, apiclient.EdituserInput{Snippets: &app})); err != nil {
				t.Fatal(err)
			}
		}
		snippets := append(slices.Clone(me.Snippets), apiclient.DomainSnippet{Name: "Agent"})
		return apiclient.EdituserInput{Snippets: &snippets}, nil
	})
	want := []apiclient.DomainSnippet{{Name: "App"}, {Name: "Agent"}}
	if err != nil || runs != 2 || !slices.Equal(me.Snippets, want) {
		t.Errorf("editMe = %+v, %v after %d runs; want %+v after 2", me.Snippets, err, runs, want)
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
