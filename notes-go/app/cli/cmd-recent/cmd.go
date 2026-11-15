package cmdrecent

import (
	"context"
	"fmt"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdeditnote "github.com/cyril-fl/tide/notes-go/app/cli/cmd-edit-note"
	cmdreadnote "github.com/cyril-fl/tide/notes-go/app/cli/cmd-read-note"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/navigator"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/screen"
	"github.com/spf13/cobra"
)

// Recent lists the last changed notes, through GET /api/recent; ↳ opens one, ^E edits it, --json prints them for
// scripts.
func Recent(cobra *cobra.Command, args []string) error {
	ctx := cobra.Context()
	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}

	if asJSON {
		notes, err := fetch(ctx)
		if err == nil {
			screen.PrintJSON(notes)
		}
		return err
	}
	load := func(navigator.Target) (navigator.Page, error) { return recentPage(ctx) }
	return navigator.Run(load, navigator.Target{}, navigator.Options{EnterOpens: true}, func(action navigator.Action, note navigator.Target) error {
		if action == navigator.ActionEdit {
			return cmdeditnote.Interactive(ctx, note.Key)
		}
		return cmdreadnote.View(ctx, note.Key)
	})
}

func fetch(ctx context.Context) ([]apiclient.ListrecentRecent, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return nil, err
	}
	response, err := client.GetApiRecentWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}

// recentPage is reloaded after an edit: the edited note moves to the top.
func recentPage(ctx context.Context) (navigator.Page, error) {
	notes, err := fetch(ctx)
	if err != nil {
		return navigator.Page{}, err
	}
	info := fmt.Sprintf("%d notes", len(notes))
	if len(notes) == 1 {
		info = "1 note"
	}
	page := navigator.Page{Title: "Recent", Path: "last changed first", Info: info, Empty: "No note yet"}
	for _, note := range notes {
		title := note.Title
		if title == "" {
			title = "untitled"
		}
		updated, err := time.Parse(time.RFC3339Nano, note.UpdatedAt)
		if err != nil {
			return navigator.Page{}, err
		}
		page.Rows = append(page.Rows, navigator.Row{
			Cells:  []string{title, note.Path, updated.Format("2 Jan 2006")},
			Target: navigator.Target{Kind: navigator.KindNote, Key: note.Id},
		})
	}
	return page, nil
}
