package cmdreplace

import (
	"fmt"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Replace replaces a plain text by another in every note, or the --folder's, through POST /api/replace as the MCP's
// replace_notes does; --dry-run prints each note's occurrences and writes nothing, --summary only the counts.
func Replace(cobra *cobra.Command, args []string) error {
	folder, err := cobra.Flags().GetString("folder")
	if err != nil {
		return err
	}
	var dryRun, summary, asJSON bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"dry-run": &dryRun, "summary": &summary, "json": &asJSON}); err != nil {
		return err
	}

	ctx := cobra.Context()
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	body := apiclient.ReplacenotesInput{Find: args[0], Replace: args[1], DryRun: &dryRun}
	if folder != "" {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, folder)
		if err != nil {
			return err
		}
		body.Folder = &id
	}
	// Every note in one transaction takes longer than Timeout.
	ctx, cancel := apiclient.Long(ctx)
	defer cancel()
	response, err := client.PostApiReplaceWithResponse(ctx, body)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}
	result := *response.JSON200
	if summary {
		result.Matches = nil
	}

	if asJSON {
		plumdebug.Json(result)
		return nil
	}
	printResult(result, args[0], args[1], dryRun)
	return nil
}

// printResult gives each note listed with its occurrences, the line before and after, then the counts.
func printResult(result apiclient.ReplacenotesResult, find, by string, dryRun bool) {
	listed := 0
	for _, match := range apiclient.Deref(result.Matches) {
		fmt.Println(match.Path)
		for _, line := range match.Occurrences {
			fmt.Printf("    %d - %s%s%s\n", line.Line, line.Before, find, line.After)
			fmt.Printf("    %d + %s%s%s\n", line.Line, line.Before, by, line.After)
			listed++
		}
	}
	if listed > 0 && listed < result.Occurrences {
		fmt.Printf("... %d more occurrence(s) not listed\n", result.Occurrences-listed)
	}

	text := fmt.Sprintf("Successfully replaced %d occurrence(s) in %d note(s)", result.Occurrences, result.Notes)
	if dryRun {
		text = fmt.Sprintf("Would replace %d occurrence(s) in %d note(s)", result.Occurrences, result.Notes)
	}
	if result.Locked > 0 {
		text += fmt.Sprintf(", %d locked note(s) left as they are", result.Locked)
	}
	if dryRun {
		text += " (dry run, nothing written)"
	}
	fmt.Println(text)
}
