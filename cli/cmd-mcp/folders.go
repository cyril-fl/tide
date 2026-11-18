package cmdmcp

import (
	"context"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// item is a note or a folder as an agent needs it, with its dates to judge whether it is still current (list_folder
// leaves them out unless asked).
type item struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	// What list_folder adds to a note when asked: tags (a pointer, so [] says "none"), length and preview.
	Tags    *[]string `json:"tags,omitempty"`
	Length  *int      `json:"length,omitempty"`
	Preview string    `json:"preview,omitempty"`
}

// folderPage is a page of the items of a folder, how many it holds in all, and whether more follow this page.
type folderPage struct {
	Items   []item `json:"items"`
	Total   int    `json:"total"`
	HasMore bool   `json:"has_more"`
}

type listFolderArgs struct {
	Folder  string `json:"folder,omitempty" jsonschema:"the folder: its uuid, or its path from the root (/Folder); the root itself when empty"`
	Limit   *int   `json:"limit,omitempty" jsonschema:"how many items at most, 50 when left out, 0 for all of them"`
	Offset  int    `json:"offset,omitempty" jsonschema:"how many items to skip, folders first then by title: the next page starts at offset + limit"`
	Dates   bool   `json:"dates,omitempty" jsonschema:"also the created_at and updated_at of each item, to judge whether it is still current"`
	Tags    bool   `json:"tags,omitempty" jsonschema:"also the tags of each note"`
	Length  bool   `json:"length,omitempty" jsonschema:"also the content length of each note, in characters"`
	Preview int    `json:"preview,omitempty" jsonschema:"also the first that many characters of each note's content, to sort a folder without read_notes"`
}

type folderArgs struct {
	Folder string `json:"folder,omitempty" jsonschema:"the folder: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type createFolderArgs struct {
	Title  string `json:"title" jsonschema:"the new folder's title"`
	Folder string `json:"folder,omitempty" jsonschema:"the folder to create it in: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type renameFolderArgs struct {
	Folder string `json:"folder" jsonschema:"the folder to rename: its uuid, or its path from the root (/Folder)"`
	Title  string `json:"title" jsonschema:"its new title"`
}

type moveArgs struct {
	Item   string `json:"item" jsonschema:"the note or folder to move: its uuid, or its path from the root (/Folder/Note)"`
	Folder string `json:"folder,omitempty" jsonschema:"the folder to move it into: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type trashedArgs struct {
	Item string `json:"item" jsonschema:"the uuid of the trashed note or folder, as list_trash gives it"`
}

func addFolderTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "list_folder",
		Annotations: reads(),
		Description: "List the notes and folders directly inside a folder, folders first then by title: the first 50, the total and has_more (true when items follow, from offset + limit), unless limit says otherwise. dates adds when each was created and changed; on each note, tags adds its tags, length its content length in characters and preview=N the first N characters of its content, to sort a folder without read_notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listFolderArgs) (*mcp.CallToolResult, folderPage, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, folderPage{}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		found, total, err := paged[[]apiclient.DomainItem](client.GetApiItems(ctx, &apiclient.GetApiItemsParams{Parent: &parentID, Limit: &limit, Offset: &offset}))
		_, items, err := made(found, err)
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

	addTool(server, readOnly, &mcp.Tool{
		Name:        "create_folder",
		Annotations: writes(false, false),
		Description: "Create a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createFolderArgs) (*mcp.CallToolResult, item, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, item{}, err
		}
		created, err := decode[apiclient.DomainItem](client.PostApiFolders(ctx, apiclient.CreatefolderInput{Title: args.Title, ParentId: &parentID}))
		return nil, one(created), err
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
		renamed, err := decode[apiclient.DomainItem](client.PatchApiFoldersId(ctx, id, apiclient.EditfolderBody{Title: &args.Title}))
		return nil, one(renamed), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "move_item",
		Annotations: writes(false, true),
		Description: "Move a note or a folder into another folder. A folder takes its content with it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args moveArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, "", args.Item)
		if err != nil {
			return nil, nil, err
		}
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]apiclient.DomainItem](client.PostApiMove(ctx, apiclient.MoveitemsInput{Ids: []string{id}, ParentId: &parentID})))
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_folder",
		Annotations: writes(true, true),
		Description: "Move a folder and everything inside it to the trash, where restore_item can take it back.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args folderArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]apiclient.DomainItem](client.DeleteApiFoldersId(ctx, id)))
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "list_trash",
		Annotations: reads(),
		Description: "List what is in the trash, last removed first, with the uuid restore_item takes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ListtrashTrashed, error) {
		trash, err := decode[[]apiclient.ListtrashTrashed](client.GetApiTrash(ctx))
		return nil, trash, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "restore_item",
		Annotations: writes(false, true),
		Description: "Take a note or a folder out of the trash, with everything removed at the same time as it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args trashedArgs) (*mcp.CallToolResult, []item, error) {
		// No resolve: a trashed item has no path to look up.
		return made(decode[[]apiclient.DomainItem](client.PostApiRestore(ctx, apiclient.RestoreBody{Ids: []string{args.Item}})))
	})
}

// one turns an item the API answers into what the agent reads.
func one(found apiclient.DomainItem) item {
	return item{ID: found.Id, Kind: string(found.Kind), Title: found.Title, CreatedAt: found.CreatedAt, UpdatedAt: editedAt(found.UpdatedAt, found.CreatedAt)}
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
		read, err := decode[[]apiclient.ReadnotesNote](client.PostApiNotesRead(ctx, apiclient.ReadnotesBody{Notes: ids[from:to], MaxChars: &maxChars}))
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
func made(found []apiclient.DomainItem, err error) (*mcp.CallToolResult, []item, error) {
	if err != nil {
		return nil, nil, err
	}
	items := make([]item, 0, len(found))
	for _, found := range found {
		items = append(items, one(found))
	}
	return nil, items, nil
}
