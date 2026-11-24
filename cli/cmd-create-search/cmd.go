package cmdcreatesearch

import (
	"errors"
	"fmt"
	"slices"

	plumdebug "github.com/cyril-fl/plum/utils/debug"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Create saves a search named by the arg with the criteria of the flags, through PATCH /api/me.
func Create(cobra *cobra.Command, args []string) error {
	search := apiclient.SavedSearch{Name: args[0]}
	if err := Criteria(cobra.Flags(), &search); err != nil {
		return err
	}
	return Save(cobra, "", search)
}

// Save creates search (ref empty) or replaces the saved search of id or name ref by it, then says so or prints it
// under --json.
func Save(cobra *cobra.Command, ref string, search apiclient.SavedSearch) error {
	asJSON, err := cobra.Flags().GetBool("json")
	if err != nil {
		return err
	}
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	saved, err := apiclient.SaveSearch(cobra.Context(), client, ref, &search)
	if err != nil {
		return err
	}
	if asJSON {
		plumdebug.Json(saved)
	} else {
		fmt.Printf("Successfully saved the search %s\n", saved.Name)
	}
	return nil
}

// Criteria sets on search the criteria whose flag was given, the others kept: what add search and edit search share.
// A blank value clears a criterion.
func Criteria(flags *pflag.FlagSet, search *apiclient.SavedSearch) error {
	var errs []error
	lists := map[string]*[]string{"tag": &search.TagsAll, "any-tag": &search.TagsAny, "no-tag": &search.TagsNone, "folder": &search.Folders}
	for name, to := range lists {
		if flags.Changed(name) {
			values, err := flags.GetStringSlice(name)
			*to, errs = slices.DeleteFunc(values, func(v string) bool { return v == "" }), append(errs, err)
		}
	}
	texts := map[string]*string{"created-from": &search.CreatedFrom, "created-to": &search.CreatedTo, "edited-from": &search.EditedFrom,
		"edited-to": &search.EditedTo, "text": &search.Text, "not": &search.NotText}
	for name, to := range texts {
		if flags.Changed(name) {
			var err error
			*to, err = flags.GetString(name)
			errs = append(errs, err)
		}
	}
	if flags.Changed("exclude-folders") {
		exclude, err := flags.GetBool("exclude-folders")
		search.FolderMode, errs = "include", append(errs, err)
		if exclude {
			search.FolderMode = "exclude"
		}
	}
	return errors.Join(errs...)
}
