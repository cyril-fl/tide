package millflagseditfolder

import (
	"context"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/editfolder"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/spf13/pflag"
)

type mill struct {
	plumprimitive.Mill
	cache

	ctx    context.Context
	client *apiclient.ClientWithResponses
	flags  *pflag.FlagSet
	id     string
}

type cache struct {
	input editfolder.Input
}

type Template struct {
	Ctx    context.Context
	Client *apiclient.ClientWithResponses
	Flags  *pflag.FlagSet
	ID     string
}

func New(params Template) *mill {
	return &mill{
		ctx:    params.Ctx,
		client: params.Client,
		flags:  params.Flags,
		id:     params.ID,
	}
}

// Resolve starts from the current folder, applies --title, or asks for it in a pre-filled form.
func (m *mill) Resolve() editfolder.Input {
	m.loadFolder()
	m.readFlags()
	m.askMissing()
	return m.cache.input
}

// Methods

// loadFolder reads the folder through GET /api/folders/{id}.
func (m *mill) loadFolder() {
	response, err := m.client.GetApiFoldersIdWithResponse(m.ctx, m.id)
	if err != nil {
		m.SetErr(err)
		return
	}
	if response.JSON200 == nil {
		m.SetErr(apiclient.Fail(response.HTTPResponse, response.Body))
		return
	}
	m.cache.input = editfolder.Input{ID: response.JSON200.Item.Id, Title: response.JSON200.Item.Title}
}

func (m *mill) readFlags() {
	if m.HasErr() || !m.flags.Changed("title") {
		return
	}

	title, err := m.flags.GetString("title")
	m.cache.input.Title = title
	m.SetErr(err)
}
