package apiclient

import (
	"context"
	"encoding/json"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listfolders"
)

// Folders is every folder with its path, through GET /api/folders, in the type huhform.PickFolder takes.
func Folders(ctx context.Context, client *ClientWithResponses) ([]listfolders.Folder, error) {
	response, err := client.GetApiFoldersWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	var folders []listfolders.Folder
	return folders, json.Unmarshal(response.Body, &folders)
}

// Tags is every tag, through GET /api/tags.
func Tags(ctx context.Context, client *ClientWithResponses) ([]domain.Tag, error) {
	response, err := client.GetApiTagsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	var tags []domain.Tag
	return tags, json.Unmarshal(response.Body, &tags)
}

// NoteTags is the tags of each note of ids by id, through POST /api/notes/tags; a note without tags is missing.
func NoteTags(ctx context.Context, client *ClientWithResponses, ids []string) (map[string][]string, error) {
	response, err := client.PostApiNotesTagsWithResponse(ctx, ReadnotetagsBody{Ids: ids})
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}
