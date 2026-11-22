package cmdmcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errBadPath = errors.New("not the relative path of a .md file")

// filesDir is exportnotes.FilesDir, where an export puts the images and attachments.
const filesDir = "_files"

// fileURL is domain.FileURL, how a note cites a file.
func fileURL(id string) string {
	return "/api/files/" + id
}

type fileArgs struct {
	File string `json:"file,omitempty" jsonschema:"a file's uuid (/api/files/<uuid>); the list when empty"`
}

type attachArgs struct {
	Name string `json:"name" jsonschema:"with its extension"`
	Data string `json:"data" jsonschema:"the bytes in base64: image, PDF, zip, audio or video"`
}

// attached is the stored file and the markdown a note cites it with.
type attached struct {
	ID       string `json:"id"`
	Markdown string `json:"markdown"`
}

type exportArgs struct {
	Folder string   `json:"folder,omitempty" jsonschema:"everything when empty"`
	Items  []string `json:"items,omitempty" jsonschema:"notes or folders, instead of folder"`
	Limit  *int     `json:"limit,omitempty" jsonschema:"50 when left out, 0 for all"`
	Offset int      `json:"offset,omitempty"`
}

// exported is a note as a markdown file with its id and last change, or a file it cites, whose bytes read_file gives.
type exported struct {
	Path      string `json:"path"`
	ID        string `json:"id,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Content   string `json:"content,omitempty"`
	File      string `json:"file,omitempty"`
}

// export is a page of the exported files, and how many there are in all.
type export struct {
	Files []exported `json:"files"`
	Total int        `json:"total"`
}

type importArgs struct {
	Files  []importFile `json:"files" jsonschema:"their directories become folders"`
	Folder string       `json:"folder,omitempty" jsonschema:"the root when empty"`
}

type importFile struct {
	Path    string `json:"path" jsonschema:"relative: Folder/Note.md"`
	Content string `json:"content"`
}

func addFileTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	// Out is any: a read answers the file as content (an image the agent sees), not as JSON.
	addTool(server, readOnly, &mcp.Tool{
		Name:        "read_file",
		Annotations: reads(),
		Description: "Read an image or attachment a note cites; without a file, list the files, largest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args fileArgs) (*mcp.CallToolResult, any, error) {
		if args.File == "" {
			files, err := decode[[]apiclient.DomainFile](client.GetApiFiles(ctx))
			return nil, struct {
				Files []apiclient.DomainFile `json:"files"`
			}{files}, err
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		response, err := client.GetApiFilesIdWithResponse(ctx, args.File)
		if err != nil {
			return nil, nil, err
		}
		if response.StatusCode() != http.StatusOK {
			return nil, nil, apiclient.Fail(response.HTTPResponse, response.Body)
		}
		content := fileContent(args.File, response.HTTPResponse.Header.Get("Content-Type"), response.Body)
		return &mcp.CallToolResult{Content: []mcp.Content{content}}, nil, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "attach_file",
		Annotations: writes(false, false),
		Description: "Store an image or attachment and answer the markdown a note cites it with.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args attachArgs) (*mcp.CallToolResult, attached, error) {
		data, err := base64.StdEncoding.DecodeString(args.Data)
		if err != nil {
			return nil, attached{}, fmt.Errorf("data is not base64: %w", err)
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		// The server reads the type from the bytes, not from this header.
		file, err := decode[apiclient.DomainFile](client.PostApiFilesWithBody(ctx, &apiclient.PostApiFilesParams{Name: &args.Name}, "application/octet-stream", bytes.NewReader(data)))
		if err != nil {
			return nil, attached{}, err
		}
		return nil, attached{ID: file.Id, Markdown: citation(file, args.Name)}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "export_notes",
		Annotations: reads(),
		Description: "Export notes as markdown files with id and last change, folders as directories.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args exportArgs) (*mcp.CallToolResult, export, error) {
		folderID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, export{}, err
		}
		ids := make([]string, 0, len(args.Items))
		for _, ref := range args.Items {
			id, err := apiclient.Resolve(ctx, client, "", ref)
			if err != nil {
				return nil, export{}, err
			}
			ids = append(ids, id)
		}

		withFiles := false // The images are only named here: their bytes would weigh down a big account's export.
		params := apiclient.GetApiExportParams{Parent: &folderID, Files: &withFiles}
		if len(ids) > 0 {
			params.Id = &ids // An empty list would still send id=.
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		response, err := client.GetApiExportWithResponse(ctx, &params)
		if err != nil {
			return nil, export{}, err
		}
		if response.StatusCode() != http.StatusOK {
			return nil, export{}, apiclient.Fail(response.HTTPResponse, response.Body)
		}
		archive, err := zip.NewReader(bytes.NewReader(response.Body), int64(len(response.Body)))
		if err != nil {
			return nil, export{}, err
		}
		files, err := exportedFiles(archive)
		// TODO: the API still builds the whole export, images left out, for each page; page it on the server if a
		// big account's export gets slow.
		return nil, export{Files: page(files, limitOf(args.Limit), max(args.Offset, 0)), Total: len(files)}, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "import_notes",
		Annotations: writes(false, false),
		Description: "Import markdown files as notes, all or nothing; a folder name already taken refuses.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args importArgs) (*mcp.CallToolResult, apiclient.ImportnotesResult, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		dir, err := importDir(args.Files)
		if err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		var body bytes.Buffer
		archive := zip.NewWriter(&body)
		if err := archive.AddFS(dir); err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		if err := archive.Close(); err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		result, err := decode[apiclient.ImportnotesResult](client.PostApiImportWithBody(ctx, &apiclient.PostApiImportParams{Parent: &parentID}, "application/zip", &body))
		return nil, result, err
	})
}

// fileContent shows an image or plays an audio to the agent; anything else comes as a resource of its type.
func fileContent(id, contentType string, data []byte) mcp.Content {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return &mcp.ImageContent{Data: data, MIMEType: contentType}
	case strings.HasPrefix(contentType, "audio/"):
		return &mcp.AudioContent{Data: data, MIMEType: contentType}
	}
	return &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: fileURL(id), MIMEType: contentType, Blob: data}}
}

// citation is the markdown of a file in a note, under the name given (the stored one is sealed): an image shown, anything else linked.
func citation(file apiclient.DomainFile, name string) string {
	link := "[" + name + "](" + fileURL(file.Id) + ")"
	if strings.HasPrefix(file.ContentType, "image/") {
		return "!" + link
	}
	return link
}

// exportedFiles reads the zip of GET /api/export: the directories left out, implied by the paths of what they hold,
// a note with its text, and a file of filesDir named by the id read_file takes, its bytes left out.
func exportedFiles(archive *zip.Reader) ([]exported, error) {
	out := []exported{}
	for _, entry := range archive.File {
		name := entry.Name
		switch {
		case strings.HasSuffix(name, "/"):
		case path.Dir(name) == filesDir && path.Ext(name) != ".md":
			// exportnotes names it <id><ext> in its FilesDir; a note is always a .md.
			out = append(out, exported{Path: name, File: strings.TrimSuffix(path.Base(name), path.Ext(name))})
		default:
			file, err := entry.Open()
			if err != nil {
				return nil, err
			}
			content, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				return nil, err
			}
			// GET /api/export gives a note's id as the entry's comment and its last change as its date.
			md := exported{Path: name, ID: entry.Comment, Content: string(content)}
			if entry.Comment != "" { // an entry without a date reads as 1979: only those with an id have one
				md.UpdatedAt = entry.Modified.UTC().Format(time.RFC3339)
			}
			out = append(out, md)
		}
	}
	return out, nil
}

// importDir makes a directory of the notes, refusing a path that would climb out of it: an agent sends text, not the
// bytes of an image.
func importDir(files []importFile) (fstest.MapFS, error) {
	dir := fstest.MapFS{}
	for _, file := range files {
		if !fs.ValidPath(file.Path) || !strings.EqualFold(path.Ext(file.Path), ".md") {
			return nil, fmt.Errorf("%w: %q", errBadPath, file.Path)
		}
		dir[file.Path] = &fstest.MapFile{Data: []byte(file.Content)}
	}
	return dir, nil
}
