package cmdmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// pageSize is how many results a list gives when the agent names no limit: a common word or a flat folder holds
// thousands of notes, megabytes an agent cannot read.
const pageSize = 50

// limitOf is the limit an agent asked, pageSize when it named none, 0 (all) for a negative one.
func limitOf(asked *int) int {
	if asked == nil {
		return pageSize
	}
	return max(*asked, 0)
}

// addTool adds tool to server, unless readOnly and the tool changes something: its annotations tell it.
func addTool[In, Out any](server *mcp.Server, readOnly bool, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if readOnly && !tool.Annotations.ReadOnlyHint {
		return
	}
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, args In) (*mcp.CallToolResult, Out, error) {
		result, out, err := handler(ctx, request, args)
		return result, out, kindHint(err)
	})
}

// kindHint names the tool for a path of the other kind, which the API only tells as text: without it an agent takes
// the path for wrong and searches for it.
func kindHint(err error) error {
	switch {
	case err == nil:
		return nil
	case strings.HasSuffix(err.Error(), "is a folder, not a note"):
		return fmt.Errorf("%w: use list_folder", err)
	case strings.HasSuffix(err.Error(), "is a note, not a folder"):
		return fmt.Errorf("%w: use read_notes", err)
	}
	return err
}

// reads marks a tool that changes nothing, so a client runs it without asking. Every tool stays in the account's notes.
func reads() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
}

// writes marks a tool that changes the notes: destructive when it deletes or overwrites (the client asks first, though
// the trash and the history keep what it took), idempotent when calling it again with the same arguments changes nothing more.
func writes(destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: idempotent, OpenWorldHint: new(false)}
}

// page is domain.Page, which the CLI cannot import: limit items from offset, all of them from offset when limit is 0.
func page[T any](list []T, limit, offset int) []T {
	offset = min(offset, len(list))
	if limit == 0 || limit > len(list)-offset {
		return list[offset:]
	}
	return list[offset : offset+limit]
}

// editedAt is the API's updated_at, which a note never edited lacks: its version is then created_at.
func editedAt(updated *string, created string) string {
	if updated == nil {
		return created
	}
	return *updated
}

// resolveFolder keeps "" as "": the API reads it as the root.
func resolveFolder(ctx context.Context, client *apiclient.ClientWithResponses, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	return apiclient.Resolve(ctx, client, apiclient.FOLDER, ref)
}
