package cmdgraph

import (
	"context"
	"fmt"
	"strings"

	cmdeditnote "github.com/cyril-fl/tide/cli/cmd-edit-note"
	cmdreadnote "github.com/cyril-fl/tide/cli/cmd-read-note"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/fold"
	"github.com/cyril-fl/tide/core/services/navigator"
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
// GET /api/graph/notes without the layout, which the CLI does not show.
func notesPage(ctx context.Context) (navigator.Page, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return navigator.Page{}, err
	}
	layout := false
	response, err := client.GetApiGraphNotesWithResponse(ctx, &apiclient.GetApiGraphNotesParams{Layout: &layout})
	if err != nil {
		return navigator.Page{}, err
	}
	if response.JSON200 == nil {
		return navigator.Page{}, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	result := response.JSON200
	fold.Sort(result.Notes, func(n apiclient.ReadgraphNote) string { return n.Path }, nil)
	linked := map[string]int{}
	for _, link := range result.Links {
		linked[link.Source]++
		linked[link.Target]++
	}
	p := navigator.Page{Title: "Graph", Path: "notes", Info: count(len(result.Notes), "note"), Empty: "No note yet"}
	for _, note := range result.Notes {
		tags := apiclient.Deref(note.Tags)
		p.Rows = append(p.Rows, navigator.Row{
			Cells:  []string{untitled(note.Title), note.Path, count(linked[note.Id], "link") + " · " + count(len(tags), "tag")},
			Tags:   tags,
			Target: navigator.Target{Kind: navigator.KindNote, Key: note.Id},
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
	result := response.JSON200
	tags, err := linkedTags(ctx, client, append(result.Links, result.Backlinks...))
	if err != nil {
		return navigator.Page{}, err
	}
	rows := append(linkedRows(result.Links, "link", tags), linkedRows(result.Backlinks, "backlink", tags)...)
	info := []string{count(len(rows)+len(result.Neighbors), "linked note")}
	if tags := apiclient.Deref(result.Note.Tags); len(tags) > 0 {
		info = append([]string{strings.Join(tags, " · ")}, info...)
	}
	p := navigator.Page{
		Title: untitled(result.Note.Item.Title),
		Path:  result.Note.Path,
		Info:  strings.Join(info, " — "),
		Empty: "No note links to or shares a tag with this one",
		Rows:  rows,
	}
	for _, neighbor := range result.Neighbors {
		p.Rows = append(p.Rows, navigator.Row{
			Cells:  []string{untitled(neighbor.Title), neighbor.Path, count(neighbor.Shared, "tag"), strings.Join(neighbor.Tags, " · ")},
			Tags:   neighbor.Tags,
			Target: navigator.Target{Kind: navigator.KindNote, Key: neighbor.Id},
		})
	}
	return p, nil
}

// linkedTags reads the tags of the linked notes: readneighbors gives them only to the neighbors.
func linkedTags(ctx context.Context, client *apiclient.ClientWithResponses, notes []apiclient.ReadneighborsLinked) (map[string][]string, error) {
	ids := make([]string, len(notes))
	for i, note := range notes {
		ids[i] = note.Id
	}
	return apiclient.NoteTags(ctx, client, ids)
}

// linkedRows lists the notes at the other end of the [[links]], same columns as the neighbors.
func linkedRows(notes []apiclient.ReadneighborsLinked, relation string, tags map[string][]string) []navigator.Row {
	rows := make([]navigator.Row, 0, len(notes))
	for _, note := range notes {
		rows = append(rows, navigator.Row{
			Cells:  []string{untitled(note.Title), note.Path, relation, ""},
			Tags:   tags[note.Id],
			Target: navigator.Target{Kind: navigator.KindNote, Key: note.Id},
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
