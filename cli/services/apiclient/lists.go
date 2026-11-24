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

// Versions is the history of a note, latest first, their texts left out, through GET /api/notes/{id}/versions.
func Versions(ctx context.Context, client *ClientWithResponses, id string) ([]DomainNoteVersion, error) {
	response, err := client.GetApiNotesIdVersionsWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}

// Version is one version of a note with its text, through GET /api/notes/{id}/versions/{version}.
func Version(ctx context.Context, client *ClientWithResponses, id, version string) (DomainNoteVersion, error) {
	response, err := client.GetApiNotesIdVersionsVersionWithResponse(ctx, id, version)
	if err != nil {
		return DomainNoteVersion{}, err
	}
	if response.JSON200 == nil {
		return DomainNoteVersion{}, Fail(response.HTTPResponse, response.Body)
	}
	return *response.JSON200, nil
}

// ReadBatch is readnotes.MaxNotes, how many notes one POST /api/notes/read takes.
const ReadBatch = 50

// ReadNotes reads the notes of refs (uuids or paths) in their order through POST /api/notes/read, ReadBatch per
// request, only the first maxChars of each content when maxChars > 0 (cut on the server, see #210).
func ReadNotes(ctx context.Context, client *ClientWithResponses, refs []string, maxChars int) ([]ReadnotesNote, error) {
	out := make([]ReadnotesNote, 0, len(refs))
	for from := 0; from < len(refs); from += ReadBatch {
		read, err := Decode[[]ReadnotesNote](client.PostApiNotesRead(ctx, ReadnotesBody{Notes: refs[from:min(from+ReadBatch, len(refs))], MaxChars: &maxChars}))
		if err != nil {
			return nil, err
		}
		out = append(out, read...)
	}
	return out, nil
}
