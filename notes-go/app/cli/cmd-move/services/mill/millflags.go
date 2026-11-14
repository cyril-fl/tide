package millflagsmove

import (
	"context"
	"errors"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/huhform"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/moveitems"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/spf13/pflag"
)

type mill struct {
	plumprimitive.Mill
	cache

	ctx    context.Context
	client *apiclient.ClientWithResponses
	flags  *pflag.FlagSet
	ids    []string
}

type cache struct {
	input          moveitems.Input
	parentFromList bool
}

type Template struct {
	Ctx    context.Context
	Client *apiclient.ClientWithResponses
	Flags  *pflag.FlagSet
	IDs    []string
}

func New(params Template) *mill {
	return &mill{
		ctx:    params.Ctx,
		client: params.Client,
		flags:  params.Flags,
		ids:    params.IDs,
	}
}

// Resolve turns the args and flags into the move; no --parent means the root.
func (m *mill) Resolve() moveitems.Input {
	m.readFlags()
	m.pickParent()
	return m.cache.input
}

// Methods

func (m *mill) readFlags() {
	var errs [2]error
	m.cache.input.IDs = m.ids
	m.cache.input.ParentID, errs[0] = m.flags.GetString("parent")
	m.cache.parentFromList, errs[1] = m.flags.GetBool("parent-from-list")
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
