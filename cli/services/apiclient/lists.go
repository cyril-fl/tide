package apiclient

import (
	"context"
)

// Folders is every folder with its path, through GET /api/folders, in the type huhform.PickFolder takes.
func Folders(ctx context.Context, client *ClientWithResponses) ([]ListfoldersFolder, error) {
	response, err := client.GetApiFoldersWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}

// Tags is every tag, through GET /api/tags.
func Tags(ctx context.Context, client *ClientWithResponses) ([]DomainTag, error) {
	response, err := client.GetApiTagsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
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

// Deref is the zero value for an optional field the API left out.
func Deref[T any](value *T) T {
	var zero T
	if value == nil {
		return zero
	}
	return *value
}

// Duplicates is the groups of notes sharing a title, or a path with samePath, through GET /api/notes/duplicates.
func Duplicates(ctx context.Context, client *ClientWithResponses, samePath bool) ([]ListduplicatesGroup, error) {
	response, err := client.GetApiNotesDuplicatesWithResponse(ctx, &GetApiNotesDuplicatesParams{SamePath: &samePath})
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}
