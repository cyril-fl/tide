package apiclient

import (
	"context"
	"io"
	"strings"
)

// FileURL is domain.FileURL, how a note cites a file.
func FileURL(id string) string {
	return "/api/files/" + id
}

// Citation is the markdown of a file in a note, under the name given (the stored one is sealed): an image shown,
// anything else linked.
func Citation(file DomainFile, name string) string {
	link := "[" + name + "](" + FileURL(file.Id) + ")"
	if strings.HasPrefix(file.ContentType, "image/") {
		return "!" + link
	}
	return link
}

// Attach stores the bytes of a file under name through POST /api/files.
func Attach(ctx context.Context, client *ClientWithResponses, name string, data io.Reader) (DomainFile, error) {
	ctx, cancel := Long(ctx)
	defer cancel()
	// The server reads the type from the bytes, not from this header.
	return Decode[DomainFile](client.PostApiFilesWithBody(ctx, &PostApiFilesParams{Name: &name}, "application/octet-stream", data))
}
