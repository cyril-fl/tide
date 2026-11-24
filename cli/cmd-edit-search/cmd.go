package cmdeditsearch

import (
	cmdcreatesearch "github.com/cyril-fl/tide/cli/cmd-create-search"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
)

// Edit changes the name and the criteria given of the saved search of id or name the arg, the others kept.
func Edit(cobra *cobra.Command, args []string) error {
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	search, err := apiclient.SavedSearchOf(cobra.Context(), client, args[0])
	if err != nil {
		return err
	}
	if cobra.Flags().Changed("name") {
		if search.Name, err = cobra.Flags().GetString("name"); err != nil {
			return err
		}
	}
	if err := cmdcreatesearch.Criteria(cobra.Flags(), &search); err != nil {
		return err
	}
	return cmdcreatesearch.Save(cobra, search.ID, search)
}
