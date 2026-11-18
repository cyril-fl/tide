package apiclient

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrNoTag is a tag name the account does not have.
var ErrNoTag = errors.New("no tag of that name")

// SearchFilter is what GET /api/search filters by, the tags by their names and the folder by its path or uuid: what
// tide search, the MCP's search_notes and tag_notes take.
type SearchFilter struct {
	Text          string
	NotText       string
	TagsAll       []string
	TagsAny       []string
	TagsNone      []string
	Folder        string
	MaxLength     *int
	Untagged      bool
	UpdatedBefore string
	EmptySections bool
}

// Filtered tells whether the filter holds a text or a filter.
func (filter SearchFilter) Filtered() bool {
	return filter.Text != "" || filter.NotText != "" || len(filter.TagsAll)+len(filter.TagsAny)+len(filter.TagsNone) > 0 ||
		filter.Folder != "" || filter.MaxLength != nil || filter.Untagged || filter.UpdatedBefore != "" || filter.EmptySections
}

// SearchParams turns the filter into the API's query, the tags and the folder by their ids, limit matches from offset
// (0: all): the one path the CLI and the MCP search by.
func SearchParams(ctx context.Context, client *ClientWithResponses, filter SearchFilter, limit, offset int) (*GetApiSearchParams, error) {
	params := &GetApiSearchParams{Q: &filter.Text, MaxLength: filter.MaxLength, Untagged: &filter.Untagged, EmptySections: &filter.EmptySections, Limit: &limit, Offset: &offset}
	if filter.UpdatedBefore != "" {
		params.UpdatedBefore = &filter.UpdatedBefore
	}
	if filter.NotText != "" {
		params.NotText = &filter.NotText
	}
	if filter.Folder != "" {
		folder, err := Resolve(ctx, client, FOLDER, filter.Folder)
		if err != nil {
			return nil, fmt.Errorf("folder %s: %w", filter.Folder, err)
		}
		params.Folder = &folder
	}
	var err error
	params.TagsAll, params.TagsAny, params.TagsNone, err = TagIDs(ctx, client, filter.TagsAll, filter.TagsAny, filter.TagsNone)
	return params, err
}

// TagIDs gives the ids of the tags named by each of the three lists (# or not, any case), nil for an empty one: a name
// unknown to the account is ErrNoTag.
func TagIDs(ctx context.Context, client *ClientWithResponses, lists ...[]string) (*[]string, *[]string, *[]string, error) {
	out := make([]*[]string, 3)
	if len(slices.Concat(lists...)) == 0 {
		return nil, nil, nil, nil
	}
	all, err := Tags(ctx, client)
	if err != nil {
		return nil, nil, nil, err
	}
	for l, names := range lists {
		if len(names) == 0 {
			continue
		}
		ids := make([]string, 0, len(names))
		for _, name := range names {
			name = strings.TrimPrefix(name, "#")
			i := slices.IndexFunc(all, func(tag DomainTag) bool { return strings.EqualFold(tag.Value, name) })
			if i < 0 {
				return nil, nil, nil, fmt.Errorf("%w: %q", ErrNoTag, name)
			}
			ids = append(ids, all[i].Id)
		}
		out[l] = &ids
	}
	return out[0], out[1], out[2], nil
}

// ErrNotesOrFilters is notes named and search filters given at once.
var ErrNotesOrFilters = errors.New("give either notes or the search filters, not both")

// Targets is the notes named, or the ids of every note the filter finds: what tide tag notes and tag_notes change.
func Targets(ctx context.Context, client *ClientWithResponses, notes []string, filter SearchFilter) ([]string, error) {
	if !filter.Filtered() {
		return notes, nil
	}
	if len(notes) > 0 {
		return nil, ErrNotesOrFilters
	}
	params, err := SearchParams(ctx, client, filter, 0, 0)
	if err != nil {
		return nil, err
	}
	response, err := client.GetApiSearchWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, Fail(response.HTTPResponse, response.Body)
	}
	for _, match := range *response.JSON200 {
		notes = append(notes, match.Id)
	}
	return notes, nil
}
