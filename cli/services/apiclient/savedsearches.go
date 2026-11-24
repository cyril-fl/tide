package apiclient

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	// ErrNoPro is an account whose plan lacks saved searches.
	ErrNoPro = errors.New("saved searches (smart folders) need the Pro plan, which this account does not have")
	// ErrNoSearch is a saved search id or name the account does not have.
	ErrNoSearch = errors.New("no saved search of that id or name")
)

// SavedSearch is a domain.SavedSearch as the CLI and the MCP read and write it: tags by name, folders by path, days
// or RFC 3339 instants.
type SavedSearch struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	TagsAll     []string `json:"tags_all,omitempty"`
	TagsAny     []string `json:"tags_any,omitempty"`
	TagsNone    []string `json:"tags_none,omitempty"`
	Folders     []string `json:"folders,omitempty"`
	FolderMode  string   `json:"folder_mode,omitempty"`
	CreatedFrom string   `json:"created_from,omitempty"`
	CreatedTo   string   `json:"created_to,omitempty"`
	EditedFrom  string   `json:"edited_from,omitempty"`
	EditedTo    string   `json:"edited_to,omitempty"`
	Text        string   `json:"text,omitempty"`
	NotText     string   `json:"not_text,omitempty"`
}

// SavedSearches is the account's saved searches, refused to a plan without them.
func SavedSearches(ctx context.Context, client *ClientWithResponses) ([]SavedSearch, error) {
	list, err := readSearches(ctx, client)
	if err != nil {
		return nil, err
	}
	names, err := searchNames(ctx, client)
	if err != nil {
		return nil, err
	}
	out := make([]SavedSearch, 0, len(list))
	for _, search := range list {
		out = append(out, names.view(search))
	}
	return out, nil
}

// SavedSearchOf is the saved search of id or name ref.
func SavedSearchOf(ctx context.Context, client *ClientWithResponses, ref string) (SavedSearch, error) {
	list, err := readSearches(ctx, client)
	if err != nil {
		return SavedSearch{}, err
	}
	i, err := findSearch(list, ref)
	if err != nil {
		return SavedSearch{}, err
	}
	names, err := searchNames(ctx, client)
	return names.view(list[i]), err
}

// RunSearch runs the saved search of id or name ref: limit of its notes from offset (0: all), and their total.
func RunSearch(ctx context.Context, client *ClientWithResponses, ref string, limit, offset int) (SavedSearch, []RunsavedsearchMatch, int, error) {
	search, err := SavedSearchOf(ctx, client, ref)
	if err != nil {
		return search, nil, 0, err
	}
	notes, total, err := Paged[[]RunsavedsearchMatch](client.GetApiSearchesId(ctx, search.ID, &GetApiSearchesIdParams{Limit: &limit, Offset: &offset}))
	return search, notes, total, err
}

// SaveSearch creates search when ref is empty, replaces the saved search of id or name ref by it (kept in its group),
// or deletes that one when search is nil, the others kept; it answers the search saved or deleted.
func SaveSearch(ctx context.Context, client *ClientWithResponses, ref string, search *SavedSearch) (SavedSearch, error) {
	if _, err := readSearches(ctx, client); err != nil {
		return SavedSearch{}, err
	}
	if search == nil && ref == "" {
		return SavedSearch{}, fmt.Errorf("%w: name the one to delete", ErrNoSearch)
	}
	var saved DomainSavedSearch
	if search != nil {
		var err error
		if saved, err = searchOf(ctx, client, *search); err != nil {
			return SavedSearch{}, err
		}
	}
	if ref == "" {
		saved.Id = rand.Text()
	}
	_, err := EditMe(ctx, client, func(me DomainUser) (EdituserInput, error) {
		list := slices.Clone(me.SavedSearches)
		if ref == "" {
			list = append(list, saved)
			return EdituserInput{SavedSearches: &list}, nil
		}
		i, err := findSearch(list, ref)
		if err != nil {
			return EdituserInput{}, err
		}
		if search == nil {
			saved = list[i]
			list = slices.Delete(list, i, i+1)
		} else {
			saved.Id, saved.ParentId = list[i].Id, list[i].ParentId // kept in its group
			list[i] = saved
		}
		return EdituserInput{SavedSearches: &list}, nil
	})
	if err != nil {
		return SavedSearch{}, err
	}
	names, err := searchNames(ctx, client)
	return names.view(saved), err
}

// readSearches reads the account's saved searches, refusing an account whose plan lacks them (as PATCH /api/me does,
// though with a clearer error).
func readSearches(ctx context.Context, client *ClientWithResponses) ([]DomainSavedSearch, error) {
	me, err := Decode[DomainUser](client.GetApiMe(ctx))
	if err != nil {
		return nil, err
	}
	if !slices.Contains(me.Features, "savedSearches") {
		return nil, ErrNoPro
	}
	return append([]DomainSavedSearch{}, me.SavedSearches...), nil
}

// findSearch gives the index of the saved search of id ref, or else of the one named ref: two of that name refuse.
func findSearch(list []DomainSavedSearch, ref string) (int, error) {
	if i := slices.IndexFunc(list, func(s DomainSavedSearch) bool { return s.Id == ref }); i >= 0 {
		return i, nil
	}
	found := -1
	for i, search := range list {
		if strings.EqualFold(search.Name, ref) {
			if found >= 0 {
				return 0, fmt.Errorf("several saved searches are named %q: give its id", ref)
			}
			found = i
		}
	}
	if found < 0 {
		return 0, fmt.Errorf("%w: %q", ErrNoSearch, ref)
	}
	return found, nil
}

// searchOf turns the criteria given into those the API stores: tags into their id, tags_all into tag_ids in "all"
// mode, folders into their uuid, a day into its first or last instant in UTC, as the app's dialog does.
func searchOf(ctx context.Context, client *ClientWithResponses, view SavedSearch) (DomainSavedSearch, error) {
	search := DomainSavedSearch{Name: strings.TrimSpace(view.Name), FolderMode: &view.FolderMode, Text: &view.Text, NotText: &view.NotText}
	var err error
	if search.TagIds, search.AnyTagIds, search.NoTagIds, err = TagIDs(ctx, client, view.TagsAll, view.TagsAny, view.TagsNone); err != nil {
		return search, err
	}
	if search.TagIds != nil {
		all := "all"
		search.TagMode = &all
	}
	if len(view.Folders) > 0 {
		ids := make([]string, 0, len(view.Folders))
		for _, ref := range view.Folders {
			id, err := Resolve(ctx, client, FOLDER, ref)
			if err != nil {
				return search, fmt.Errorf("%s: %w", ref, err)
			}
			ids = append(ids, id)
		}
		search.FolderIds = &ids
	}
	for _, date := range []struct {
		value string
		end   bool
		to    **string
	}{{view.CreatedFrom, false, &search.CreatedFrom}, {view.CreatedTo, true, &search.CreatedTo}, {view.EditedFrom, false, &search.EditedFrom}, {view.EditedTo, true, &search.EditedTo}} {
		instant, err := instantOf(date.value, date.end)
		if err != nil {
			return search, err
		}
		*date.to = instant
	}
	return search, nil
}

// instantOf reads a day (its first instant, or its last when end) or an RFC 3339 instant; nil when empty.
func instantOf(value string, end bool) (*string, error) {
	if value == "" {
		return nil, nil
	}
	if day, err := time.Parse(time.DateOnly, value); err == nil {
		if end {
			day = day.Add(24*time.Hour - time.Millisecond)
		}
		instant := day.Format(time.RFC3339Nano)
		return &instant, nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return nil, fmt.Errorf("%q is neither a day (2026-01-31) nor an RFC 3339 instant", value)
	}
	return &value, nil
}

// names gives the tags and folders of saved searches by name and path, an id left as it is once its tag or folder is gone.
type names map[string]string

func searchNames(ctx context.Context, client *ClientWithResponses) (names, error) {
	tags, err := Decode[[]DomainTag](client.GetApiTags(ctx))
	if err != nil {
		return nil, err
	}
	folders, err := Decode[[]ListfoldersFolder](client.GetApiFolders(ctx))
	if err != nil {
		return nil, err
	}
	out := names{}
	for _, tag := range tags {
		out[tag.Id] = tag.Value
	}
	for _, folder := range folders {
		out[folder.Id] = folder.Path
	}
	return out, nil
}

func (n names) of(ids *[]string) []string {
	out := make([]string, 0, len(Deref(ids)))
	for _, id := range Deref(ids) {
		out = append(out, cmp.Or(n[id], id))
	}
	return out
}

// view turns a saved search the API stores into what a client reads.
func (n names) view(search DomainSavedSearch) SavedSearch {
	// tag_ids are all of them in "all" mode, one of them in "any" (an older search): the API refuses any_tag_ids beside
	// the latter.
	tagsAll, tagsAny := n.of(search.TagIds), []string{}
	if Deref(search.TagMode) != "all" {
		tagsAll, tagsAny = []string{}, tagsAll
	}
	return SavedSearch{
		ID:          search.Id,
		Name:        search.Name,
		TagsAll:     tagsAll,
		TagsAny:     append(tagsAny, n.of(search.AnyTagIds)...),
		TagsNone:    n.of(search.NoTagIds),
		Folders:     n.of(search.FolderIds),
		FolderMode:  Deref(search.FolderMode),
		CreatedFrom: Deref(search.CreatedFrom),
		CreatedTo:   Deref(search.CreatedTo),
		EditedFrom:  Deref(search.EditedFrom),
		EditedTo:    Deref(search.EditedTo),
		Text:        Deref(search.Text),
		NotText:     Deref(search.NotText),
	}
}
