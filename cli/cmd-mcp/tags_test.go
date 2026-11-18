package cmdmcp

import (
	"context"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestTagsHistoryLinks reads what surrounds a note: its tags, its versions, its links.
func TestTagsHistoryLinks(t *testing.T) {
	agent := connect(t)

	var target, source note
	call(t, agent, "create_item", map[string]any{"content": "# Target\n#go"}, &target)
	call(t, agent, "create_item", map[string]any{"content": "# Source\nsee [[Target]] and [[Nowhere]] #go"}, &source)

	call(t, agent, "create_item", map[string]any{"content": "# Other\n#golf #rust"}, nil)
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
	var preview, summary struct {
		Value   string `json:"value"`
		Changed *int   `json:"changed"`
		Notes   []struct {
			Path string   `json:"path"`
			Diff []string `json:"diff"`
		} `json:"notes"`
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "go", "to": "golang", "dry_run": true}, &preview)
	if preview.Value != "golang" || len(preview.Notes) == 0 || len(preview.Notes[0].Diff) == 0 {
		t.Errorf("rename_tag dry_run = %+v", preview)
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "go", "to": "golang", "dry_run": true, "summary": true}, &summary)
	if summary.Changed == nil || *summary.Changed != len(preview.Notes) || len(summary.Notes) != 0 {
		t.Errorf("rename_tag summary = %+v, want %d notes counted, none listed", summary, len(preview.Notes))
	}
	var still tags
	call(t, agent, "tags", map[string]any{"tag": "go", "limit": 1}, &still)
	if still.Total == 0 {
		t.Errorf("rename_tag dry_run renamed the tag: %+v", still)
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "go", "to": "golang"}, &renamed)
	// A name taken refuses, unless merged.
	call(t, agent, "create_item", map[string]any{"content": "# Merged\n#gl"}, nil)
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
	// dry_run counts the notes and writes nothing: the real call then deletes the tag.
	var deleted, dry, kept, brief deletedTag
	var bare note
	call(t, agent, "create_item", map[string]any{"content": "#golang"}, &bare)
	call(t, agent, "delete_tag", map[string]any{"tag": "golang", "dry_run": true, "keep_empty": true}, &kept)
	call(t, agent, "tags", map[string]any{"tag": "golang", "limit": 1}, &still)
	if len(kept.Tags) != 1 || kept.Changed == 0 || kept.Trashed != 0 || len(kept.Notes) != 0 || still.Total == 0 {
		t.Errorf("delete_tag dry_run keep_empty = %+v, tag left with %d notes", kept, still.Total)
	}
	// bare holds nothing but the tag: trashed, listed unless summary.
	call(t, agent, "delete_tag", map[string]any{"tag": "golang", "dry_run": true}, &dry)
	call(t, agent, "delete_tag", map[string]any{"tag": "golang", "dry_run": true, "summary": true}, &brief)
	if dry.Trashed != 1 || len(dry.Notes) != 1 || dry.Notes[0].Id != bare.ID || brief.Trashed != 1 || len(brief.Notes) != 0 || brief.Changed != dry.Changed {
		t.Errorf("delete_tag dry_run = %+v, summary = %+v", dry, brief)
	}
	call(t, agent, "delete_tag", map[string]any{"tag": "golang"}, &deleted)
	if renamed.Value != "golang" || len(deleted.Tags) != 1 || deleted.Changed != dry.Changed {
		t.Errorf("rename_tag = %+v, delete_tag = %+v", renamed, deleted)
	}
}
