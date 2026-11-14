package cmdgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cmdeditnote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-edit-note"
	cmdreadnote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-read-note"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/navigator"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readgraph"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readneighbors"
	"github.com/spf13/cobra"
)

// kindNotes is the start page: every note.
const kindNotes = "notes"

// Graph walks the notes through their [[links]] and the tags they share, from a note (or from every note).
func Graph(cobra *cobra.Command, args []string) error {
	ctx := cobra.Context()
	start := navigator.Target{Kind: kindNotes}
	if len(args) == 1 {
		start = navigator.Target{Kind: navigator.KindNote, Key: args[0]}
	}

	load := func(target navigator.Target) (navigator.Page, error) { return loadPage(ctx, target) }
	return navigator.Run(load, start, navigator.Options{}, func(action navigator.Action, note navigator.Target) error {
		if action == navigator.ActionEdit {
			return cmdeditnote.Interactive(ctx, note.Key)
		}
		return cmdreadnote.View(ctx, note.Key)
	})
}

// loadPage builds what a node shows: every note, or the neighbors of a note.
func loadPage(ctx context.Context, target navigator.Target) (navigator.Page, error) {
	if target.Kind == navigator.KindNote {
		return notePage(ctx, target.Key)
	}
	return notesPage(ctx)
}

// notesPage lists every note with how many [[links]] and tags join it to the others, like the web graph, through
// GET /api/graph/notes.
// TODO: the route also lays the graph out (and saves the layout when the graph changed), which the CLI does not show,
// add a way to skip it if it slows the CLI down on a big account.
func notesPage(ctx context.Context) (navigator.Page, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return navigator.Page{}, err
	}
	response, err := client.GetApiGraphNotesWithResponse(ctx)
	if err != nil {
		return navigator.Page{}, err
	}
	if response.JSON200 == nil {
		return navigator.Page{}, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	// Decoded into the processes' types, not the generated ones: the pages stay those of the processes.
	var result readgraph.Result
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return navigator.Page{}, err
	}
	domain.SortFold(result.Notes, func(n readgraph.Note) string { return n.Path }, nil)
	linked := map[string]int{}
	for _, link := range result.Links {
		linked[link.Source]++
		linked[link.Target]++
	}
	p := navigator.Page{Title: "Graph", Path: "notes", Info: count(len(result.Notes), "note"), Empty: "No note yet"}
	for _, note := range result.Notes {
		p.Rows = append(p.Rows, navigator.Row{
			Cells:  []string{untitled(string(note.Title)), note.Path, count(linked[note.ID], "link") + " · " + count(len(note.Tags), "tag")},
			Tags:   note.Tags,
			Target: navigator.Target{Kind: navigator.KindNote, Key: note.ID},
		})
	}
	return p, nil
}

// notePage reads the note and its neighbors through GET /api/notes/{id}/neighbors.
func notePage(ctx context.Context, id string) (navigator.Page, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return navigator.Page{}, err
	}
	response, err := client.GetApiNotesIdNeighborsWithResponse(ctx, id)
	if err != nil {
		return navigator.Page{}, err
	}
	if response.JSON200 == nil {
		return navigator.Page{}, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	var result readneighbors.Result
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return navigator.Page{}, err
	}
	tags, err := linkedTags(ctx, client, append(result.Links, result.Backlinks...))
	if err != nil {
		return navigator.Page{}, err
	}
	rows := append(linkedRows(result.Links, "link", tags), linkedRows(result.Backlinks, "backlink", tags)...)
	info := []string{count(len(rows)+len(result.Neighbors), "linked note")}
	if len(result.Note.Tags) > 0 {
		info = append([]string{strings.Join(result.Note.Tags, " · ")}, info...)
	}
	p := navigator.Page{
		Title: untitled(string(result.Note.Item.Title)),
		Path:  result.Note.Path,
		Info:  strings.Join(info, " — "),
		Empty: "No note links to or shares a tag with this one",
		Rows:  rows,
	}
	for _, neighbor := range result.Neighbors {
		p.Rows = append(p.Rows, navigator.Row{
			Cells:  []string{untitled(string(neighbor.Title)), neighbor.Path, count(neighbor.Shared, "tag"), strings.Join(neighbor.Tags, " · ")},
			Tags:   neighbor.Tags,
			Target: navigator.Target{Kind: navigator.KindNote, Key: neighbor.ID},
		})
	}
	return p, nil
}

// linkedTags reads the tags of the linked notes: readneighbors gives them only to the neighbors.
func linkedTags(ctx context.Context, client *apiclient.ClientWithResponses, notes []readneighbors.Linked) (map[string][]string, error) {
	ids := make([]string, len(notes))
	for i, note := range notes {
		ids[i] = note.ID
	}
	return apiclient.NoteTags(ctx, client, ids)
}

// linkedRows lists the notes at the other end of the [[links]], same columns as the neighbors.
func linkedRows(notes []readneighbors.Linked, relation string, tags map[string][]string) []navigator.Row {
	rows := make([]navigator.Row, 0, len(notes))
	for _, note := range notes {
		rows = append(rows, navigator.Row{
			Cells:  []string{untitled(string(note.Title)), note.Path, relation, ""},
			Tags:   tags[note.ID],
			Target: navigator.Target{Kind: navigator.KindNote, Key: note.ID},
		})
	}
	return rows
}

// Helpers

func untitled(title string) string {
	if title == "" {
		return "untitled"
	}
	return title
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
