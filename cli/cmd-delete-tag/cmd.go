package cmddeletetag

import (
	"fmt"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Delete deletes tags, not the notes carrying them, in one DELETE /api/tags; an unknown one is skipped. --keep-empty
// keeps out of the trash a note left empty, --dry-run prints the notes it would trash and writes nothing.
func Delete(cobra *cobra.Command, args []string) error {
	var dryRun, keepEmpty bool
	if err := plumcliutilsargs.GetBoolFlags(cobra, map[string]*bool{"dry-run": &dryRun, "keep-empty": &keepEmpty}); err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	response, err := client.DeleteApiTagsWithResponse(cobra.Context(), apiclient.DeletetagBody{Values: args, DryRun: &dryRun, KeepEmpty: &keepEmpty})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		return apiclient.Fail(response.HTTPResponse, response.Body)
	}

	result := response.JSON200
	for _, note := range result.Trashed {
		fmt.Println(note.Path)
	}
	if dryRun {
		fmt.Printf("Would delete %d tag(s): %d note(s) changed, %d to the trash (dry run, nothing written)\n", len(result.Tags), result.Changed, len(result.Trashed))
		return nil
	}
	fmt.Printf("Successfully deleted %d tag(s): %d note(s) changed, %d moved to the trash\n", len(result.Tags), result.Changed, len(result.Trashed))
	return nil
}
