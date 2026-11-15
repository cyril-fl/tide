package huhform

import (
	"charm.land/huh/v2"
	"github.com/cyril-fl/tide/cli/services/apiclient"
)

// PickFolder asks for a folder in a list of paths ("/" filters it) and returns its id ("" = the root).
func PickFolder(folders []apiclient.ListfoldersFolder) (string, error) {
	id := ""
	picker := huh.NewSelect[string]().Title("Folder").Options(folderOptions(folders)...).Value(&id)
	return id, New(picker).Run()
}

// folderOptions shows paths, the root first; the value is the folder id.
func folderOptions(folders []apiclient.ListfoldersFolder) []huh.Option[string] {
	options := []huh.Option[string]{huh.NewOption("/", "")}
	for _, folder := range folders {
		options = append(options, huh.NewOption(folder.Path, folder.Id))
	}
	return options
}
