package millflagscreatefolder

import (
	"context"
	"errors"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/huhform"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/spf13/pflag"
)

type mill struct {
	plumprimitive.Mill
	cache

	ctx    context.Context
	client *apiclient.ClientWithResponses
	flags  *pflag.FlagSet
}

// Input is the folder to create, for POST /api/folders; ParentID "" is the root.
type Input struct {
	Title    string
	ParentID string
}

type cache struct {
	input          Input
	parentFromList bool
}

type Template struct {
	Ctx    context.Context
	Client *apiclient.ClientWithResponses
	Flags  *pflag.FlagSet
}

func New(params Template) *mill {
	return &mill{
		ctx:    params.Ctx,
		client: params.Client,
		flags:  params.Flags,
	}
}

// Resolve turns the flags into a complete folder, asking in a form for what is missing.
func (m *mill) Resolve() Input {
	m.readFlags()
	m.pickParent()
	m.askMissing()

	return m.cache.input
}

// Methods

func (m *mill) readFlags() {
	var errs [3]error

	m.cache.input.Title, errs[0] = m.flags.GetString("title")
	m.cache.input.ParentID, errs[1] = m.flags.GetString("parent")
	m.cache.parentFromList, errs[2] = m.flags.GetBool("parent-from-list")

	m.SetErr(errors.Join(errs[:]...))
}

// pickParent asks for the parent in the folder list with --parent-from-list.
func (m *mill) pickParent() {
	if m.HasErr() || !m.cache.parentFromList {
		return
	}

	folders, err := apiclient.Folders(m.ctx, m.client)
	if err != nil {
		m.SetErr(err)
		return
	}

	id, err := huhform.PickFolder(folders)
	m.cache.input.ParentID = id
	m.SetErr(err)
}
