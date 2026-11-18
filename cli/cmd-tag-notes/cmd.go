package cmdtagnotes

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	plumcliconfig "github.com/cyril-fl/plum/cli/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	cmdsearch "github.com/cyril-fl/tide/cli/cmd-search"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/screen"
	"github.com/spf13/cobra"
)

var (
	errNoChange = errors.New("give --add or --remove")
	errNoTarget = errors.New("give notes or a filter of search")
)

// Flags is the flags of tag notes then the filters of search, declared once in its config.
func Flags() []plumcliconfig.Cmdflags {
	filters := slices.DeleteFunc(slices.Clone(config.Current.Commands.Search.Flags), func(flag plumcliconfig.Cmdflags) bool {
		return slices.Contains([]string{"limit", "json", "duplicates", "same-path"}, flag.Name)
	})
	return slices.Concat(config.Current.Commands.TagNotes.Flags, filters)
}

// Tag adds and removes tags on the notes named or found by the filters of tide search, through PATCH /api/notes/tags
// as the MCP's tag_notes does; --dry-run prints each note's diff and writes nothing, --summary only the counts and the
// notes that failed.
func Tag(cobra *cobra.Command, args []string) error {
	ctx := cobra.Context()
	flags := cobra.Flags()
	text, err := flags.GetString("text")
	if err != nil {
		return err
	}
	filter, err := cmdsearch.FilterOf(cobra, text)
	if err != nil {
		return err
	}
	add, err := flags.GetStringSlice("add")
	if err != nil {
		return err
	}
	remove, err := flags.GetStringSlice("remove")
	if err != nil {
		return err
	}
	var dryRun, asJSON, summary bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"dry-run": &dryRun, "json": &asJSON, "summary": &summary}); err != nil {
		return err
	}
	if len(add)+len(remove) == 0 {
		return errNoChange
	}
	if len(args) == 0 && !filter.Filtered() {
		return errNoTarget
	}

	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	notes, err := apiclient.Targets(ctx, client, args, filter)
	if err != nil {
		return err
	}
	tagged := []apiclient.TagnotesTagged{}
	if len(notes) > 0 {
		// Hundreds of notes take longer than Timeout.
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		response, err := client.PatchApiNotesTagsWithResponse(ctx, apiclient.TagnotesInput{Notes: notes, Add: &add, Remove: &remove, DryRun: &dryRun})
		if err != nil {
			return err
		}
		if response.JSON200 == nil {
			return apiclient.Fail(response.HTTPResponse, response.Body)
		}
		tagged = *response.JSON200
	}

	switch {
	case asJSON && summary:
		plumdebug.Json(summarize(tagged))
	case asJSON:
		screen.PrintJSON(tagged)
	default:
		printTagged(tagged, dryRun, summary)
	}
	return nil
}

// counts is the answer of --summary --json: the counts, then the notes that failed.
type counts struct {
	Changed   int                        `json:"changed"`
	Unchanged int                        `json:"unchanged"`
	Failed    int                        `json:"failed"`
	Notes     []apiclient.TagnotesTagged `json:"notes"`
}

// summarize counts the notes failed, changed (a tag added or removed) and the rest, and keeps only the failed ones.
func summarize(tagged []apiclient.TagnotesTagged) counts {
	out := counts{Notes: []apiclient.TagnotesTagged{}}
	for _, note := range tagged {
		switch {
		case note.Error != nil:
			out.Failed++
			out.Notes = append(out.Notes, note)
		case len(apiclient.Deref(note.Added))+len(apiclient.Deref(note.Removed)) > 0:
			out.Changed++
		default:
			out.Unchanged++
		}
	}
	return out
}

// printTagged gives each note with what happened to the tags asked and, with --dry-run, the lines that change, then the
// counts; with summary, only the notes that failed, then the counts.
func printTagged(tagged []apiclient.TagnotesTagged, dryRun, summary bool) {
	for _, note := range tagged {
		if summary && note.Error == nil {
			continue
		}
		name := apiclient.Deref(note.Path)
		if name == "" {
			name = note.Note
		}
		fmt.Println(name)
		if note.Error != nil {
			fmt.Printf("  error: %s\n", *note.Error)
			continue
		}
		added, removed := apiclient.Deref(note.Added), apiclient.Deref(note.Removed)
		for _, line := range []struct {
			label string
			tags  []string
		}{{"added", added}, {"already there", apiclient.Deref(note.AlreadyPresent)}, {"removed", removed}, {"absent", apiclient.Deref(note.NotPresent)}} {
			if len(line.tags) > 0 {
				fmt.Printf("  %s: %s\n", line.label, strings.Join(line.tags, ", "))
			}
		}
		for _, line := range apiclient.Deref(note.Diff) {
			fmt.Printf("    %s\n", line)
		}
	}
	total := summarize(tagged)
	line := fmt.Sprintf("%d changed, %d unchanged, %d failed", total.Changed, total.Unchanged, total.Failed)
	if dryRun {
		line += " (dry run, nothing written)"
	}
	fmt.Println(line)
}
