package cmdedittag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/huh/v2"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/huhform"
	"github.com/spf13/cobra"
)

var errRequired = errors.New("required")

// Edit renames the tag through PATCH /api/tags/{value}, in the text of its notes too; --merge folds it into a tag
// already there, --dry-run prints the notes it would rewrite and their diff, --summary only their count.
func Edit(cobra *cobra.Command, args []string) error {
	var merge, dryRun, summary bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"merge": &merge, "dry-run": &dryRun, "summary": &summary}); err != nil {
		return err
	}
	_, err := rename(cobra.Context(), args[0], args[1], merge, dryRun, summary)
	return err
}

// Interactive renames the tag through a form pre-filled with its value, and returns the new value.
func Interactive(ctx context.Context, value string) (string, error) {
	to := value
	if err := huhform.New(huh.NewInput().Title("Tag").Value(&to).Validate(notBlank)).Run(); err != nil {
		return value, err
	}
	return rename(ctx, value, to, false, false, false)
}

func rename(ctx context.Context, from string, to string, merge, dryRun, summary bool) (string, error) {
	client, err := apiclient.Read().Client()
	if err != nil {
		return from, err
	}
	response, err := client.PatchApiTagsValueWithResponse(ctx, from, apiclient.EdittagBody{Value: to, Merge: &merge, DryRun: &dryRun})
	if err != nil {
		return from, err
	}
	if response.JSON200 == nil {
		return from, apiclient.Fail(response.HTTPResponse, response.Body)
	}

	if dryRun {
		notes := apiclient.Deref(response.JSON200.Notes)
		for _, note := range notes {
			if summary {
				break
			}
			fmt.Println(note.Path)
			for _, line := range note.Diff {
				fmt.Printf("    %s\n", line)
			}
		}
		fmt.Printf("Would rename %q to %q in %d note(s) (dry run, nothing written)\n", from, response.JSON200.Value, len(notes))
		return from, nil
	}
	fmt.Printf("Successfully renamed %q to %q\n", from, response.JSON200.Value)
	return response.JSON200.Value, nil
}

func notBlank(value string) error {
	if strings.TrimSpace(value) == "" {
		return errRequired
	}

	return nil
}
