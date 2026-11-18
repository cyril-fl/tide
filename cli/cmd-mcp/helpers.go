package cmdmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// pageSize is how many results a list gives when the agent names no limit: a common word or a flat folder holds
// thousands of notes, megabytes an agent cannot read.
const pageSize = 50

// readBatch is readnotes.MaxNotes, how many notes one read_notes request takes.
const readBatch = 50

// limitOf is the limit an agent asked, pageSize when it named none, 0 (all) for a negative one.
func limitOf(asked *int) int {
	if asked == nil {
		return pageSize
	}
	return max(*asked, 0)
}

// errAccountStale is the text of edituser.ErrStale the API answers: the account changed since it was read.
const errAccountStale = "account settings changed elsewhere since they were read"

// editMe reads the account, has edit build the whole lists to save from it, and saves them through PATCH /api/me with
// the updated_at read: a write of the app or another agent in between is read again and edit run over it, once.
func editMe(ctx context.Context, client *apiclient.ClientWithResponses, edit func(apiclient.DomainUser) (apiclient.EdituserInput, error)) (apiclient.DomainUser, error) {
	for try := 0; ; try++ {
		me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
		if err != nil {
			return me, err
		}
		input, err := edit(me)
		if err != nil {
			return me, err
		}
		input.UpdatedAt = &me.UpdatedAt
		me, err = decode[apiclient.DomainUser](client.PatchApiMe(ctx, input))
		if err == nil || err.Error() != errAccountStale || try > 0 {
			return me, err
		}
	}
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
		return fmt.Errorf("%w: use read_note", err)
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

// decode reads a JSON answer into the generated type of the route; any status but 2xx is the API's error.
func decode[T any](response *http.Response, err error) (T, error) {
	var out T
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return out, err
	}
	if response.StatusCode/100 != 2 {
		return out, apiclient.Fail(response, body)
	}
	return out, json.Unmarshal(body, &out)
}

// paged decodes a page of a list, and the X-Total-Count of the whole list.
func paged[T any](response *http.Response, err error) (T, int, error) {
	out, err := decode[T](response, err)
	if err != nil {
		return out, 0, err
	}
	total, err := strconv.Atoi(response.Header.Get("X-Total-Count"))
	return out, total, err
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
