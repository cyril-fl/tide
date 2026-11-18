package cmdsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdeditnote "github.com/cyril-fl/tide/cli/cmd-edit-note"
	cmdreadnote "github.com/cyril-fl/tide/cli/cmd-read-note"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/navigator"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/spf13/cobra"
)

var (
	errNoFilter  = errors.New("give a text or a filter")
	errListAlone = errors.New("--duplicates and --same-path take no text, filter nor --limit")
)

// Search lists the notes containing the text in their title or content and matching the filters, --limit of them
// (0: all), through GET /api/search; ↳ opens one, ^E edits it, --json prints them for scripts.
func Search(cobra *cobra.Command, args []string) error {
	ctx := cobra.Context()
	filter, err := FilterOf(cobra, strings.Join(args, " "))
	if err != nil {
		return err
	}
	var asJSON, duplicates, samePath bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"json": &asJSON, "duplicates": &duplicates, "same-path": &samePath}); err != nil {
		return err
	}
	if duplicates || samePath {
		if filter.Filtered() || cobra.Flags().Changed("limit") {
			return errListAlone
		}
		return printDuplicates(ctx, samePath, asJSON)
	}
	if !filter.Filtered() {
		return errNoFilter
	}
	limit, err := cobra.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	if limit < 0 {
		return fmt.Errorf("--limit is 0 or more, got %d", limit)
	}

	if asJSON {
		matches, _, err := fetch(ctx, filter, limit)
		if err == nil {
			screen.PrintJSON(matches)
		}
		return err
	}
	load := func(navigator.Target) (navigator.Page, error) { return searchPage(ctx, filter, limit) }
	return navigator.Run(load, navigator.Target{}, navigator.Options{EnterOpens: true}, func(action navigator.Action, note navigator.Target) error {
		if action == navigator.ActionEdit {
			return cmdeditnote.Interactive(ctx, note.Key)
		}
		return cmdreadnote.View(ctx, note.Key)
	})
}

// FilterOf reads the text and the filter flags of tide search, which tide tag notes shares, --max-length 0 being no filter as for the API.
func FilterOf(cobra *cobra.Command, text string) (apiclient.SearchFilter, error) {
	flags := cobra.Flags()
	filter := apiclient.SearchFilter{Text: text}
	errs := make([]error, 8)
	filter.TagsAll, errs[0] = flags.GetStringSlice("tag")
	filter.TagsAny, errs[1] = flags.GetStringSlice("any-tag")
	filter.TagsNone, errs[2] = flags.GetStringSlice("no-tag")
	filter.Folder, errs[3] = flags.GetString("folder")
	filter.NotText, errs[4] = flags.GetString("not")
	filter.Untagged, errs[5] = flags.GetBool("untagged")
	filter.UpdatedBefore, errs[6] = flags.GetString("updated-before")
	if maxLength, err := flags.GetInt("max-length"); err != nil || maxLength > 0 {
		filter.MaxLength, errs[7] = &maxLength, err
	}
	return filter, errors.Join(errs...)
}

// fetch gives the first page of matches and their total (X-Total-Count).
func fetch(ctx context.Context, filter apiclient.SearchFilter, limit int) ([]apiclient.SearchnotesMatch, int, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return nil, 0, err
	}
	params, err := apiclient.SearchParams(ctx, client, filter, limit, 0)
	if err != nil {
		return nil, 0, err
	}
	response, err := client.GetApiSearchWithResponse(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	if response.JSON200 == nil {
		return nil, 0, apiclient.Fail(response.HTTPResponse, response.Body)
	}
	matches := *response.JSON200
	total := len(matches)
	if response.Headers200 != nil && response.Headers200.XTotalCount != nil {
		total = *response.Headers200.XTotalCount
	}
	return matches, total, nil
}

// searchPage is reloaded after an edit: the note may not match anymore.
func searchPage(ctx context.Context, filter apiclient.SearchFilter, limit int) (navigator.Page, error) {
	matches, total, err := fetch(ctx, filter, limit)
	if err != nil {
		return navigator.Page{}, err
	}
	info := fmt.Sprintf("%d notes", len(matches))
	if len(matches) == 1 {
		info = "1 note"
	}
	// The total is about right past the limit: the notes after it are not opened.
	if total > len(matches) {
		info = fmt.Sprintf("%d of about %d notes (--limit)", len(matches), total)
	}
	page := navigator.Page{Title: "Search", Path: fmt.Sprintf("%q", filter.Text), Info: info, Empty: "No note matches"}
	for _, match := range matches {
		title := match.Title
		if title == "" {
			title = "untitled"
		}
		page.Rows = append(page.Rows, navigator.Row{
			Cells:  []string{title, match.Path, match.Snippet},
			Target: navigator.Target{Kind: navigator.KindNote, Key: match.Id},
		})
	}
	return page, nil
}

// printDuplicates prints the groups of notes sharing a title, or a path with samePath: the title or path, then each
// note's id, path, date and length.
func printDuplicates(ctx context.Context, samePath, asJSON bool) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	groups, err := apiclient.Duplicates(ctx, client, samePath)
	if err != nil {
		return err
	}
	if asJSON {
		screen.PrintJSON(groups)
		return nil
	}
	if len(groups) == 0 {
		fmt.Println("No duplicate")
	}
	for i, group := range groups {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(group.Key)
		for _, note := range group.Notes {
			updated, err := time.Parse(time.RFC3339Nano, note.UpdatedAt)
			if err != nil {
				return err
			}
			fmt.Printf("  %s  %s  %s  %d chars\n", note.Id, note.Path, updated.Format("2 Jan 2006"), note.Length)
		}
	}
	return nil
}
