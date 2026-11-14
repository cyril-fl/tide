package cmdsearch

import (
	"context"
	"fmt"
	"strings"

	cmdeditnote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-edit-note"
	cmdreadnote "github.com/cyril-fl/notes/notes-go/app/cli/cmd-read-note"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/navigator"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/postgrey"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/searchnotes"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/screen"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"
)

// Search lists the notes containing the text in their title or content, --limit of them (0: all); ↳ opens one,
// ^E edits it, --json prints them for scripts.
func Search(cobra *cobra.Command, args []string) error {
	ctx := cobra.Context()
	text := strings.Join(args, " ")

	var asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON}); err != nil {
		return err
	}
	limit, err := cobra.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	if limit < 0 {
		return fmt.Errorf("--limit is 0 or more, got %d", limit)
	}
	if asJSON {
		process := searchnotes.Process(ctx, postgrey.GetDb(), text).Page(limit, 0)
		process.Run()
		if !process.HasErr() {
			screen.PrintJSON(process.GetResult())
		}
		return process.GetErr()
	}

	load := func(navigator.Target) (navigator.Page, error) { return searchPage(ctx, text, limit) }
	return navigator.Run(load, navigator.Target{}, navigator.Options{EnterOpens: true}, func(action navigator.Action, note navigator.Target) error {
		if action == navigator.ActionEdit {
			return cmdeditnote.Interactive(ctx, note.Key)
		}
		return cmdreadnote.View(ctx, note.Key)
	})
}

// searchPage is reloaded after an edit: the note may not match anymore.
func searchPage(ctx context.Context, text string, limit int) (navigator.Page, error) {
	process := searchnotes.Process(ctx, postgrey.GetDb(), text).Page(limit, 0)
	process.Run()

	matches := process.GetResult()
	info := fmt.Sprintf("%d notes", len(matches))
	if len(matches) == 1 {
		info = "1 note"
	}
	// The total is about right past the limit: the notes after it are not opened.
	if total := process.GetTotal(); total > len(matches) {
		info = fmt.Sprintf("%d of about %d notes (--limit)", len(matches), total)
	}
	page := navigator.Page{Title: "Search", Path: fmt.Sprintf("%q", text), Info: info, Empty: "No note matches"}
	for _, match := range matches {
		title := match.Title
		if title == "" {
			title = "untitled"
		}
		page.Rows = append(page.Rows, navigator.Row{
			Cells:  []string{title, match.Path, match.Snippet},
			Target: navigator.Target{Kind: navigator.KindNote, Key: match.ID},
		})
	}
	return page, process.GetErr()
}
