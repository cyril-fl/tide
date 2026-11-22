package cmdmcp

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type savedSearchesArgs struct {
	Search string `json:"search,omitempty" jsonschema:"its id or name; the list when empty"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"50 when left out, 0 for all"`
	Offset int    `json:"offset,omitempty"`
}

type saveSearchArgs struct {
	Search      string   `json:"search,omitempty" jsonschema:"the one to replace or delete, by id or name; a new one when empty"`
	Delete      bool     `json:"delete,omitempty" jsonschema:"remove it instead, its notes kept"`
	Name        string   `json:"name,omitempty"`
	TagsAll     []string `json:"tags_all,omitempty"`
	TagsAny     []string `json:"tags_any,omitempty"`
	TagsNone    []string `json:"tags_none,omitempty"`
	Folders     []string `json:"folders,omitempty" jsonschema:"with their subfolders"`
	FolderMode  string   `json:"folder_mode,omitempty" jsonschema:"include (default) or exclude the folders' notes"`
	CreatedFrom string   `json:"created_from,omitempty" jsonschema:"a day or RFC 3339 instant, as the other dates"`
	CreatedTo   string   `json:"created_to,omitempty"`
	EditedFrom  string   `json:"edited_from,omitempty"`
	EditedTo    string   `json:"edited_to,omitempty"`
	Text        string   `json:"text,omitempty"`
	NotText     string   `json:"not_text,omitempty"`
}

// savedSearch is a domain.SavedSearch as an agent reads it: tags by name, folders by path.
type savedSearch struct {
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

// searches is the account's saved searches, or one run with a page of its notes and their total.
type searches struct {
	Searches []savedSearch                   `json:"searches,omitempty"`
	Search   *savedSearch                    `json:"search,omitempty"`
	Notes    []apiclient.RunsavedsearchMatch `json:"notes,omitempty"`
	Total    int                             `json:"total,omitempty"`
}

var (
	errNoPro    = errors.New("saved searches (smart folders) need the Pro plan, which this account does not have")
	errNoSearch = errors.New("no saved search of that id or name")
)

func addSavedSearchTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "saved_searches",
		Annotations: reads(),
		Description: "List the saved searches (the app's smart folders), or run one: its notes and total; Pro plan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args savedSearchesArgs) (*mcp.CallToolResult, searches, error) {
		list, err := readSearches(ctx, client)
		if err != nil {
			return nil, searches{}, err
		}
		names, err := searchNames(ctx, client)
		if err != nil {
			return nil, searches{}, err
		}
		if args.Search == "" {
			out := make([]savedSearch, 0, len(list))
			for _, search := range list {
				out = append(out, names.view(search))
			}
			return nil, searches{Searches: out}, nil
		}
		i, err := findSearch(list, args.Search)
		if err != nil {
			return nil, searches{}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		notes, total, err := paged[[]apiclient.RunsavedsearchMatch](client.GetApiSearchesId(ctx, list[i].Id, &apiclient.GetApiSearchesIdParams{Limit: &limit, Offset: &offset}))
		if err != nil {
			return nil, searches{}, err
		}
		view := names.view(list[i])
		return nil, searches{Search: &view, Notes: notes, Total: total}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "save_search",
		Annotations: writes(true, false),
		Description: "Create, replace or delete a saved search (its notes match every criterion), the others kept; Pro plan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args saveSearchArgs) (*mcp.CallToolResult, savedSearch, error) {
		if _, err := readSearches(ctx, client); err != nil {
			return nil, savedSearch{}, err
		}
		if args.Delete && args.Search == "" {
			return nil, savedSearch{}, fmt.Errorf("%w: give search to delete", errNoSearch)
		}
		var search apiclient.DomainSavedSearch
		if !args.Delete {
			var err error
			if search, err = searchOf(ctx, client, args); err != nil {
				return nil, savedSearch{}, err
			}
		}
		if args.Search == "" {
			search.Id = rand.Text()
		}
		_, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			list := slices.Clone(me.SavedSearches)
			if args.Search == "" {
				list = append(list, search)
				return apiclient.EdituserInput{SavedSearches: &list}, nil
			}
			i, err := findSearch(list, args.Search)
			if err != nil {
				return apiclient.EdituserInput{}, err
			}
			if args.Delete {
				search = list[i]
				list = slices.Delete(list, i, i+1)
			} else {
				search.Id = list[i].Id
				list[i] = search
			}
			return apiclient.EdituserInput{SavedSearches: &list}, nil
		})
		if err != nil {
			return nil, savedSearch{}, err
		}
		names, err := searchNames(ctx, client)
		return nil, names.view(search), err
	})
}

// readSearches reads the account's saved searches, refusing an account whose plan lacks them (as PATCH /api/me does,
// though with a clearer error for an agent).
func readSearches(ctx context.Context, client *apiclient.ClientWithResponses) ([]apiclient.DomainSavedSearch, error) {
	me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
	if err != nil {
		return nil, err
	}
	if !slices.Contains(me.Features, "savedSearches") {
		return nil, errNoPro
	}
	return append([]apiclient.DomainSavedSearch{}, me.SavedSearches...), nil
}

// findSearch gives the index of the saved search of id ref, or else of the one named ref: two of that name refuse.
func findSearch(list []apiclient.DomainSavedSearch, ref string) (int, error) {
	if i := slices.IndexFunc(list, func(s apiclient.DomainSavedSearch) bool { return s.Id == ref }); i >= 0 {
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
		return 0, fmt.Errorf("%w: %q", errNoSearch, ref)
	}
	return found, nil
}

// searchOf turns the criteria an agent gives into those the API stores: tags into their id, tags_all into tag_ids in
// "all" mode, folders into their uuid, a day into its first or last instant in UTC, as the app's dialog does.
func searchOf(ctx context.Context, client *apiclient.ClientWithResponses, args saveSearchArgs) (apiclient.DomainSavedSearch, error) {
	search := apiclient.DomainSavedSearch{Name: strings.TrimSpace(args.Name), FolderMode: &args.FolderMode, Text: &args.Text, NotText: &args.NotText}
	var err error
	if search.TagIds, search.AnyTagIds, search.NoTagIds, err = apiclient.TagIDs(ctx, client, args.TagsAll, args.TagsAny, args.TagsNone); err != nil {
		return search, err
	}
	if search.TagIds != nil {
		all := "all"
		search.TagMode = &all
	}
	if len(args.Folders) > 0 {
		ids := make([]string, 0, len(args.Folders))
		for _, ref := range args.Folders {
			id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, ref)
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
	}{{args.CreatedFrom, false, &search.CreatedFrom}, {args.CreatedTo, true, &search.CreatedTo}, {args.EditedFrom, false, &search.EditedFrom}, {args.EditedTo, true, &search.EditedTo}} {
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

func searchNames(ctx context.Context, client *apiclient.ClientWithResponses) (names, error) {
	tags, err := decode[[]apiclient.DomainTag](client.GetApiTags(ctx))
	if err != nil {
		return nil, err
	}
	folders, err := decode[[]apiclient.ListfoldersFolder](client.GetApiFolders(ctx))
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
	out := make([]string, 0, len(apiclient.Deref(ids)))
	for _, id := range apiclient.Deref(ids) {
		out = append(out, cmp.Or(n[id], id))
	}
	return out
}

// view turns a saved search the API stores into what the agent reads.
func (n names) view(search apiclient.DomainSavedSearch) savedSearch {
	// tag_ids are all of them in "all" mode, one of them in "any" (an older search): the API refuses any_tag_ids beside
	// the latter.
	tagsAll, tagsAny := n.of(search.TagIds), []string{}
	if apiclient.Deref(search.TagMode) != "all" {
		tagsAll, tagsAny = []string{}, tagsAll
	}
	return savedSearch{
		ID:          search.Id,
		Name:        search.Name,
		TagsAll:     tagsAll,
		TagsAny:     append(tagsAny, n.of(search.AnyTagIds)...),
		TagsNone:    n.of(search.NoTagIds),
		Folders:     n.of(search.FolderIds),
		FolderMode:  apiclient.Deref(search.FolderMode),
		CreatedFrom: apiclient.Deref(search.CreatedFrom),
		CreatedTo:   apiclient.Deref(search.CreatedTo),
		EditedFrom:  apiclient.Deref(search.EditedFrom),
		EditedTo:    apiclient.Deref(search.EditedTo),
		Text:        apiclient.Deref(search.Text),
		NotText:     apiclient.Deref(search.NotText),
	}
}
