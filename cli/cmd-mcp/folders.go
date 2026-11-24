package cmdmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	errCreateMode = errors.New("give one of content (a note), notes (several) or title (a folder)")
	errTrashAlone = errors.New("trash takes no folder")
)

// item is a note or a folder as an agent needs it, with its dates to judge whether it is still current (list_folder
// leaves them out unless asked).
type item struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Readonly  bool   `json:"readonly,omitempty"` // a locked note
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	// What list_folder adds to a note when asked: tags (a pointer, so [] says "none"), length and preview.
	Tags    *[]string `json:"tags,omitempty"`
	Length  *int      `json:"length,omitempty"`
	Preview string    `json:"preview,omitempty"`
}

// changed is what a move, a deletion or a restore reached: an object, as MCP wants of a tool's output.
type changed struct {
	Items []item `json:"items"`
}

// folderPage is a page of the items of a folder, how many it holds in all, and whether more follow this page; or
// what the trash holds.
type folderPage struct {
	Items   []item                       `json:"items,omitempty"`
	Trash   []apiclient.ListtrashTrashed `json:"trash,omitempty"`
	Total   int                          `json:"total"`
	HasMore bool                         `json:"has_more"`
}

type listFolderArgs struct {
	Folder  string `json:"folder,omitempty" jsonschema:"the root when empty"`
	Limit   *int   `json:"limit,omitempty" jsonschema:"default 50, 0 for all"`
	Offset  int    `json:"offset,omitempty"`
	Dates   bool   `json:"dates,omitempty" jsonschema:"add created_at and updated_at"`
	Tags    bool   `json:"tags,omitempty" jsonschema:"add each note's tags"`
	Length  bool   `json:"length,omitempty" jsonschema:"add each note's length in characters"`
	Preview int    `json:"preview,omitempty" jsonschema:"add each note's first N characters"`
	Trash   bool   `json:"trash,omitempty" jsonschema:"the trash instead, last removed first"`
}

type createArgs struct {
	Content string   `json:"content,omitempty" jsonschema:"a note's markdown: its first heading is the title, its #hashtags tags"`
	Notes   []string `json:"notes,omitempty" jsonschema:"several contents at once"`
	Title   string   `json:"title,omitempty" jsonschema:"a folder's title, instead of content"`
	Folder  string   `json:"folder,omitempty" jsonschema:"where; the root when empty"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags added to content"`
}

type renameFolderArgs struct {
	Folder string `json:"folder"`
	Title  string `json:"title"`
}

type moveArgs struct {
	Items  []string `json:"items" jsonschema:"notes or folders"`
	Folder string   `json:"folder,omitempty" jsonschema:"the root when empty"`
}

type deleteArgs struct {
	Items     []string `json:"items,omitempty"`
	Recursive bool     `json:"recursive,omitempty" jsonschema:"for folders"`
}

type trashedArgs struct {
	Items []string `json:"items" jsonschema:"uuids, as list_folder trash gives them"`
}

func addFolderTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "list_folder",
		Annotations: reads(),
		Description: "List a folder's notes and folders, folders first, with total; or the trash.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listFolderArgs) (*mcp.CallToolResult, folderPage, error) {
		if args.Trash {
			if args.Folder != "" {
				return nil, folderPage{}, errTrashAlone
			}
			trash, err := apiclient.Decode[[]apiclient.ListtrashTrashed](client.GetApiTrash(ctx))
			return nil, folderPage{Trash: trash, Total: len(trash)}, err
		}
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, folderPage{}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		found, total, err := apiclient.Paged[[]apiclient.DomainItem](client.GetApiItems(ctx, &apiclient.GetApiItemsParams{Parent: &parentID, Limit: &limit, Offset: &offset}))
		_, page, err := made(found, err)
		items := page.Items
		if !args.Dates {
			for i := range items {
				items[i].CreatedAt, items[i].UpdatedAt = "", ""
			}
		}
		if err == nil && (args.Tags || args.Length || args.Preview > 0) {
			err = describe(ctx, client, items, args)
		}
		return nil, folderPage{Items: items, Total: total, HasMore: offset+len(items) < total}, err
	})

	// Out is any: a note is answered with its path and tags, a folder as an item.
	addTool(server, readOnly, &mcp.Tool{
		Name:        "create_item",
		Annotations: writes(false, false),
		Description: "Create a note from content, several from notes, or a folder from title.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createArgs) (*mcp.CallToolResult, any, error) {
		if len(slices.DeleteFunc([]bool{args.Content != "", len(args.Notes) > 0, args.Title != ""}, func(set bool) bool { return !set })) != 1 {
			return nil, nil, errCreateMode
		}
		if args.Content != "" {
			args.Notes = []string{args.Content}
		}
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		if args.Title != "" {
			created, err := apiclient.Decode[apiclient.DomainItem](client.PostApiFolders(ctx, apiclient.CreatefolderInput{Title: args.Title, ParentId: &parentID}))
			if err != nil {
				return nil, nil, err
			}
			return nil, one(created), nil
		}
		read := make([]note, 0, len(args.Notes))
		for _, content := range args.Notes {
			created, err := apiclient.Decode[apiclient.DomainItem](client.PostApiNotes(ctx, apiclient.CreatenoteInput{Content: content, ParentId: &parentID, Tags: &args.Tags}))
			if err == nil {
				// Read back: the API answers the item alone, without the path nor the tags it parsed.
				var got note
				if got, err = readNote(apiclient.Decode[apiclient.ReaditemResult](client.GetApiNotesId(ctx, created.Id))); err == nil {
					read = append(read, got)
					continue
				}
			}
			// No batch endpoint: the notes made before the failure stay, say which.
			made := make([]string, len(read))
			for i, n := range read {
				made[i] = n.Path
			}
			return nil, nil, fmt.Errorf("note %d of %d: %w (created before it: %v)", len(read)+1, len(args.Notes), err, made)
		}
		if args.Content != "" {
			return nil, read[0], nil
		}
		return nil, factor(read), nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "rename_folder",
		Annotations: writes(false, true),
		Description: "Change the title of a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameFolderArgs) (*mcp.CallToolResult, item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, item{}, err
		}
		renamed, err := apiclient.Decode[apiclient.DomainItem](client.PatchApiFoldersId(ctx, id, apiclient.EditfolderBody{Title: &args.Title}))
		return nil, one(renamed), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "move_item",
		Annotations: writes(false, true),
		Description: "Move notes or folders, with their content, into another folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args moveArgs) (*mcp.CallToolResult, changed, error) {
		ids := make([]string, len(args.Items))
		for i, it := range args.Items {
			id, err := apiclient.Resolve(ctx, client, "", it)
			if err != nil {
				return nil, changed{}, err
			}
			ids[i] = id
		}
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, changed{}, err
		}
		return made(apiclient.Decode[[]apiclient.DomainItem](client.PostApiMove(ctx, apiclient.MoveitemsInput{Ids: ids, ParentId: &parentID})))
	})

	// A folder needs recursive: a folder's path given for a note must not send a whole subtree to the trash.
	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_item",
		Annotations: writes(true, true),
		Description: "Trash notes, or with recursive folders and content.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args deleteArgs) (*mcp.CallToolResult, changed, error) {
		kind, other, hint := apiclient.NOTE, "is a folder, not a note", "give recursive to trash it with its content"
		if args.Recursive {
			kind, other, hint = apiclient.FOLDER, "is a note, not a folder", "leave out recursive"
		}
		ids := make([]string, len(args.Items))
		for i, it := range args.Items {
			id, err := apiclient.Resolve(ctx, client, kind, it)
			if err != nil {
				if strings.HasSuffix(err.Error(), other) {
					return nil, changed{}, fmt.Errorf("%w: %s", err, hint)
				}
				return nil, changed{}, err
			}
			ids[i] = id
		}
		if args.Recursive {
			return made(apiclient.Decode[[]apiclient.DomainItem](client.DeleteApiFolders(ctx, apiclient.DeleteApiFoldersJSONRequestBody{Ids: ids})))
		}
		return made(apiclient.Decode[[]apiclient.DomainItem](client.DeleteApiNotes(ctx, apiclient.DeleteApiNotesJSONRequestBody{Ids: ids})))
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "restore_item",
		Annotations: writes(false, true),
		Description: "Take notes or folders out of the trash, with what was trashed with them.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args trashedArgs) (*mcp.CallToolResult, changed, error) {
		// No resolve: a trashed item has no path to look up.
		return made(apiclient.Decode[[]apiclient.DomainItem](client.PostApiRestore(ctx, apiclient.RestoreBody{Ids: args.Items})))
	})
}

// one turns an item the API answers into what the agent reads.
func one(found apiclient.DomainItem) item {
	return item{ID: found.Id, Kind: string(found.Kind), Title: found.Title, Readonly: apiclient.Deref(found.Readonly), CreatedAt: found.CreatedAt, UpdatedAt: editedAt(found.UpdatedAt, found.CreatedAt)}
}

// describe adds the tags, length and preview asked to the notes of a page, by read_notes batches (the cut is made on the
// server, see #210): the content is read there whatever is asked, but only what was asked travels.
func describe(ctx context.Context, client *apiclient.ClientWithResponses, items []item, args listFolderArgs) error {
	var at []int // the notes' places in items
	var ids []string
	for i, it := range items {
		if it.Kind == string(apiclient.NOTE) {
			at, ids = append(at, i), append(ids, it.ID)
		}
	}
	maxChars := max(args.Preview, 1) // 0 would send the whole content
	for from := 0; from < len(ids); from += readBatch {
		to := min(from+readBatch, len(ids))
		read, err := apiclient.Decode[[]apiclient.ReadnotesNote](client.PostApiNotesRead(ctx, apiclient.ReadnotesBody{Notes: ids[from:to], MaxChars: &maxChars}))
		if err != nil {
			return err
		}
		for j, r := range read {
			it := &items[at[from+j]]
			if args.Tags {
				tags := append([]string{}, apiclient.Deref(r.Tags)...)
				it.Tags = &tags
			}
			if args.Length {
				it.Length = &r.Length
			}
			if args.Preview > 0 {
				it.Preview = apiclient.Deref(r.Content)
			}
		}
	}
	return nil
}

// made turns the items of an answer into what the agent reads, for a tool answering them.
func made(found []apiclient.DomainItem, err error) (*mcp.CallToolResult, changed, error) {
	if err != nil {
		return nil, changed{}, err
	}
	items := make([]item, 0, len(found))
	for _, found := range found {
		items = append(items, one(found))
	}
	return nil, changed{items}, nil
}
